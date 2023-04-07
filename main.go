// this  program shows a centralized UI for remote parsing of logs supported by the log-parse-agent program.
// it allows listing of all apps supported by agents, allows submit log grep/tail requests,
// and parse the output in browser UI.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

var validate *validator.Validate

// app config properties
type AppConfig struct {
	Server                    string `json:"server"`
	Port                      string `json:"port"`
	AgentCacheRefreshInterval int    `json:"agent-cache-refresh-interval-minutes"`
	UIUserName                string `json:"ui-username"`
	UIPassword                string `json:"ui-password"`
}

var appConfig AppConfig

// initializer
func init() {
	//read app config
	var content []byte
	var err error
	if content, err = os.ReadFile("./config/app-config.json"); err != nil {
		panic(err)
	}
	if err = json.Unmarshal(content, &appConfig); err != nil {
		panic(err)
	}

	//schedule a task to cleanup dead Agent info,
	//an agent is considered dead if no heartbeat info received in configured interval.
	go func() {
		for {
			time.Sleep(time.Duration(appConfig.AgentCacheRefreshInterval) * time.Minute)
			logAgentConfigMap.Range(func(key, value interface{}) bool {
				diff := time.Since(value.(LogAgentConfig).UpdateTime)
				if diff.Minutes() > float64(appConfig.AgentCacheRefreshInterval+1) {
					logAgentConfigMap.Delete(key)
				}
				return true
			})
		}
	}()

	//validator framework
	validate = validator.New()

	gin.SetMode(gin.ReleaseMode)
}

// program entry point
func main() {
	router := gin.Default()
	router.LoadHTMLGlob("./html/*.html")

	//api without authentication
	router.POST("/api/logs/search/agent/info", logSearchAgentConfigHandler)

	//api with authentication
	authorized := router.Group("/", gin.BasicAuth(gin.Accounts{
		appConfig.UIUserName: appConfig.UIPassword,
	}))
	authorized.GET("/", homePageHandler)
	authorized.GET("/api/logs/search", logSearchFormHandler)
	authorized.POST("/api/logs/search/files", validateLogOperationRequest, logSearchFilesHandler)
	authorized.POST("/api/logs/search/lines", validateLogOperationRequest, logSearchLinesHandler)
	authorized.POST("/api/logs/tail/files", logTailFilesHandler)
	authorized.POST("/api/logs/command/cancel", commandCancelHandler)

	log.Printf("***********log-parse-ui UI Starting at:%s:%s\n", appConfig.Server, appConfig.Port)
	router.Run(appConfig.Server + ":" + appConfig.Port)
}

// home page handler
func homePageHandler(c *gin.Context) {
	//prepare the APP listing
	appMap := make(map[string]struct{})
	logAgentConfigMap.Range(func(key, value interface{}) bool {
		for _, v := range value.(LogAgentConfig).AppsSupported {
			appMap[v.App] = struct{}{}
		}
		return true
	})

	//sort the apps listing
	keys := make([]string, 0, len(appMap))
	for k := range appMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	c.HTML(http.StatusOK, "app_info.html", gin.H{
		"Apps": keys,
	})
}

// log search form handler
func logSearchFormHandler(c *gin.Context) {
	app := strings.TrimSpace(c.Query("APP"))
	if errs := validate.Var(app, "required"); errs != nil {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(errs.Error()))
		return
	}

	nodes := []string{}
	logs := []string{}
	tmpLogs := make(map[string]struct{})

	//check if App and User is supported
	logAgentConfigMap.Range(func(key, value interface{}) bool {
		agentConfigVal := value.(LogAgentConfig)
		for _, appConfigVal := range agentConfigVal.AppsSupported {
			if strings.EqualFold(app, appConfigVal.App) {
				nodes = append(nodes, agentConfigVal.AgentHost)
				for _, tmp := range appConfigVal.Logs {
					tmpLogs[tmp] = struct{}{}
				}
			}
		}
		return true
	})

	for k, _ := range tmpLogs {
		logs = append(logs, k)
	}

	//build "node log" list
	nodeLogs := []string{}
	for _, vNode := range nodes {
		for _, vLog := range logs {
			nodeLogs = append(nodeLogs, strings.Join([]string{vNode, vLog}, " "))
		}
	}

	//render
	if len(nodes) > 0 {
		c.HTML(http.StatusOK, "log-search-form.html", gin.H{
			"APP": app, "NODES": nodes, "LOGS": logs, "LOGSTOTAIL": nodeLogs,
		})
	} else {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte("<html><body>USER or APP Not Supported!</body></html>"))
	}
}

// struct for data posted by agents
type appConfiguration struct {
	App               string   `json:"app"`
	SearchTimeout     uint     `json:"search-timeout-minute"`
	PreMatchLinesMax  uint     `json:"pre-match-lines-max"`
	PostMatchLinesMax uint     `json:"post-match-lines-max"`
	SearchDuration    uint     `json:"search-duration"`
	Logs              []string `json:"logs"`
	Active            bool     `json:"active"`
	AllowDownload     bool     `json:"allow-download"`
}

// struct to capture logAgent info
type LogAgentConfig struct {
	UpdateTime     time.Time          `json:"update-time"`
	AgentHost      string             `json:"agent-host"`
	AgentPort      string             `json:"agent-port"`
	UsersSupported []string           `json:"users-supported"`
	AppsSupported  []appConfiguration `json:"apps-supported"`
}

// synchronized map
var logAgentConfigMap sync.Map

// handler to receive and process agent post info
func logSearchAgentConfigHandler(c *gin.Context) {

	log.Printf("Agent Config Post received...\n")
	var agentConfig LogAgentConfig
	err := json.NewDecoder(c.Request.Body).Decode(&agentConfig)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Message Rejected"})
		log.Printf("Error receiving Agent Info:%s", err)
	} else if !strings.EqualFold(agentConfig.AgentHost, "") {
		log.Printf("Accepted Agent Info:%v", agentConfig)

		//update Map
		agentConfig.UpdateTime = time.Now()
		logAgentConfigMap.Store(agentConfig.AgentHost, agentConfig)

		//response
		c.JSON(http.StatusOK, gin.H{"message": "Message Accepted"})
		if entry, ok := logAgentConfigMap.Load(agentConfig.AgentHost); ok {
			log.Printf("Processed Agent Info:%v\n", entry)
		}
	}
}

type logAPIOutput struct {
	Node  string   `json:"node"`
	Data  []string `json:"data"`
	Error string   `json:"error"`
}

// log search files handler
func logSearchFilesHandler(c *gin.Context) {
	var fileOPArray []logAPIOutput
	data, exist := c.Get("InputData")
	if !exist {
		return
	}
	inputData := data.(logOperationInput)

	//build post body
	postVal := url.Values{}
	postVal.Set("search-app", inputData.App)
	postVal.Add("search-text", inputData.SearchText)
	postVal.Add("isRegEx", strconv.FormatBool(inputData.IsRegEx))
	postVal.Add("pre-match-lines", strconv.Itoa(inputData.PreMatchLines))
	postVal.Add("post-match-lines", strconv.Itoa(inputData.PostMatchLines))
	for _, v := range inputData.LogFiles {
		postVal.Add("search-files", v)
	}

	fileOPArray = logAPICall(false, "logs/search/files", postVal, inputData.Nodes, nil)
	c.HTML(http.StatusOK, "log-search-res1.html", gin.H{
		"APP": inputData.App, "SEARCHTEXT": inputData.SearchText, "FILEOUTPUTS": fileOPArray, "PREMATCHLINES": inputData.PostMatchLines, "POSTMATCHLINES": inputData.PostMatchLines, "ISREGEX": inputData.IsRegEx,
	})
}

// log search lines handler
func logSearchLinesHandler(c *gin.Context) {
	data, exist := c.Get("InputData")
	if !exist {
		return
	}
	inputData := data.(logOperationInput)

	//build post body
	postVal := url.Values{}
	postVal.Set("search-app", inputData.App)
	postVal.Add("search-text", inputData.SearchText)
	postVal.Add("isRegEx", strconv.FormatBool(inputData.IsRegEx))
	postVal.Add("pre-match-lines", strconv.Itoa(inputData.PreMatchLines))
	postVal.Add("post-match-lines", strconv.Itoa(inputData.PostMatchLines))
	for _, v := range inputData.LogFiles {
		postVal.Add("search-files", v)
	}

	logAPICall(true, "logs/search/lines", postVal, inputData.Nodes, c)
}

// log tail handler
func logTailFilesHandler(c *gin.Context) {
	//parse input & validate
	app := c.Request.FormValue("app")
	nodeSPACElog := strings.TrimSpace(c.Request.FormValue("nodeSPACElog"))
	if !strings.EqualFold(nodeSPACElog, "") {
		tmp := strings.Split(nodeSPACElog, " ")
		nodes := []string{tmp[0]}
		log := tmp[1]

		//build post body
		postVal := url.Values{}
		postVal.Set("search-app", app)
		postVal.Add("search-file", log)
		logAPICall(true, "logs/tail/files", postVal, nodes, c)
	}
}

// log operation cancel handler
func commandCancelHandler(c *gin.Context) {
	cmdKey := c.Request.FormValue("cmd-key")
	node := c.Request.FormValue("node")
	if err := validate.Var(cmdKey, "required"); err == nil {
		if err = validate.Var(node, "required"); err == nil {
			postVal := url.Values{}
			postVal.Set("cmd-key", cmdKey)
			url := "http://" + node + ":" + getAgentPortByHost(node) + "/api/logs/command/cancel"
			op, err := httpPOSTExecute(url, postVal)
			if err != nil {
				log.Printf("Error on making API Call %s, Error:%s \n", url, err.Error())
			} else {
				log.Printf("Success on making API Call %s, Response:%s \n", url, string(op))
			}
		}
	}
}

// utility method
func getAgentPortByHost(agentHost string) string {
	if value, ok := logAgentConfigMap.Load(agentHost); ok {
		return value.(LogAgentConfig).AgentPort
	}
	return "9998" //default value
}

// struct for API input
type logOperationInput struct {
	App            string   `json:"app" validate:"required"`
	SearchText     string   `json:"search-text" validate:"required"`
	IsRegEx        bool     `json:"is-regex"`
	PreMatchLines  int      `json:"pre-match-lines" validate:"gte=0,lte=9"`
	PostMatchLines int      `json:"post-match-lines" validate:"gte=0,lte=9"`
	Nodes          []string `json:"nodes" validate:"required,min=1"`
	LogFiles       []string `json:"logs" validate:"required,min=1"`
}

// validator handler
func validateLogOperationRequest(c *gin.Context) {
	//parse request input
	app := c.Request.FormValue("app")
	searchText := c.Request.FormValue("search-text")
	isRegEx := c.Request.FormValue("is-reg-ex")
	tmpRegEx := false
	if strings.EqualFold(isRegEx, "regex") {
		tmpRegEx = true
	}
	preMatchLines := c.Request.FormValue("pre-match-lines")
	postMatchLines := c.Request.FormValue("post-match-lines")
	preMatchTmp, _ := strconv.Atoi(preMatchLines)
	postMatchTmp, _ := strconv.Atoi(postMatchLines)
	nodes := c.Request.Form["nodes"]
	logs := c.Request.Form["logs"]

	inputData := logOperationInput{App: app, SearchText: searchText, IsRegEx: tmpRegEx, PreMatchLines: preMatchTmp, PostMatchLines: postMatchTmp, Nodes: nodes, LogFiles: logs}

	//perform validation
	if errs := validate.Struct(inputData); errs != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": errs.Error()})
		log.Printf("API:%s, InputData:%v, Validation Failed:%s \n", c.Request.URL.Path, inputData, errs.Error())
		c.Abort()
		return
	}

	c.Set("InputData", inputData)
}

// utility method - API call
func logAPICall(renderOutput bool, apiContext string, postVal url.Values, nodes []string, c *gin.Context) []logAPIOutput {
	//make request for each node
	var wg sync.WaitGroup
	outputData := make([]logAPIOutput, len(nodes))
	for i, node := range nodes {
		wg.Add(1)

		//slice append is not concurrency safe, so using index to update slice
		//each go routine will update it's own index in slice
		go func(index int, node string) {
			defer wg.Done()
			var resData []byte
			var err error
			url := "http://" + node + ":" + getAgentPortByHost(node) + "/api/" + apiContext

			var output logAPIOutput
			output.Node = node
			if renderOutput {
				err := httpPOSTExecuteAndRender(url, postVal, c)
				if err != nil {
					output.Error = err.Error()
				}
			} else {
				if resData, err = httpPOSTExecute(url, postVal); err != nil {
					output.Error = err.Error()
				} else {
					if err = json.Unmarshal(resData, &output); err != nil {
						output.Error = err.Error()
					}
				}
			}
			outputData[index] = output

			if renderOutput && !strings.EqualFold(output.Error, "") {
				c.JSON(http.StatusTooManyRequests, gin.H{"error": output.Error})
			}
		}(i, node)

	}

	wg.Wait()
	return outputData
}

// utility method - http post call
func httpPOSTExecute(urlStr string, v url.Values) ([]byte, error) {

	r, err := http.PostForm(urlStr, v)
	if err != nil {
		return []byte{}, err
	}
	defer r.Body.Close()

	if r.StatusCode == http.StatusTooManyRequests {
		return nil, errors.New("agent returned too many requests error, please wait for previous operations to complete")
	}

	bodyText, err := io.ReadAll(r.Body)
	if err != nil {
		return []byte{}, err
	}

	return bodyText, nil
}

// utility method - http post call
func httpPOSTExecuteAndRender(urlStr string, v url.Values, c *gin.Context) error {
	r, err := http.PostForm(urlStr, v)
	if err != nil {
		return err
	}
	defer r.Body.Close()

	if r.StatusCode == http.StatusTooManyRequests {
		return errors.New("agent returned too many requests error, please wait for previous operations to complete")
	}

	urlParsed, err := url.Parse(urlStr)
	if err != nil {
		return err
	}
	scanner := bufio.NewScanner(r.Body)
	scanner.Split(bufio.ScanLines)
	parsefirstLine := true
	for scanner.Scan() {
		//first line is always cmdKey
		if parsefirstLine {
			cmdKey := scanner.Text()
			c.HTML(http.StatusOK, "text_output.html", gin.H{"KEY": cmdKey, "NODE": urlParsed.Hostname()})
			parsefirstLine = false
			continue
		}

		line := scanner.Text()
		c.HTML(http.StatusOK, "blank.html", gin.H{"BODY": template.HTML(line) + "\n"})
		c.Writer.Flush()
	}

	return nil
}
