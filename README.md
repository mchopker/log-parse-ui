# log-parse-ui

log-parse-ui is a Golang program that provides a central UI to allow querying grep and tail operations on the remote logs managed by [log-parse-agent](https://github.com/mchopker/log-parse-agent) programs.

## Description

log-parse-ui shows in its UI all applications and logs posted by all [log-parse-agent](https://github.com/mchopker/log-parse-agent) programs that are posting their configurations into this log-parse-ui server. The user can select the application and logs for which he wants to perform grep or tail operation, the grep and tail output is shown in the log-parse-ui UI.

## Getting Started

### Dependencies

You would need Golang 1.20 or above to build and run this program.

### Clone the project

```
$ git clone https://github.com/mchopker/log-parse-ui
$ cd log-parse-ui
```

### Build and Run

```
$ go build
$ chmod 755 log-parse-ui
$ ./log-parse-ui
```

### Usage

The log-parse-ui program reads its config file ./config/app-config.json on startup and exposes its UI at http://SERVER-HOST:SERVER-POST/. The host and port at which UI would run can be configured inside ./config/app-config.json file. 

The following is the default configuration exists in the ./config/app-config.json file:

```json

{
    "server":"127.0.0.1",
    "port":"9997",
    "agent-cache-refresh-interval-minutes":1,
    "ui-username":"mchopker",
    "ui-password":"Mahesh@123"
}

```

The following is the explanation of the various attributes allowed in the  ./config/app-config.json file:

| Attribute                               | Mandatory / Optional | Purpose                                                                                                                                                                                                                                                                                                            |
| :-------------------------------------- | :------------------- | :----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| server                              | Mandatory            | The IPAddress of the host machine where the log-parse-ui UI will be running. The default value is 127.0.0.1, change it to real IP if you want UI to be called remotely.                                                                                                                                           |
| port                              | Mandatory            | The port where log-parse-ui UI will be exposed.                                                                                                                                                                                                                                                                   |
| agent-cache-refresh-interval-minutes                              | Mandatory             | The internal at which the configuration posted by [log-parse-agent](https://github.com/mchopker/log-parse-agent) would be invalidated. This is to clean up any agent's configuration which came initially but the agent is not running now.                                                                                                                                                 |
| ui-username                              | Mandatory             | The login username for the log-parse-ui UI. |
| ui-password                              | Mandatory             | The login password for the log-parse-ui UI. |


### Testing

With the default configuration the UI will be running in http://127.0.0.1:9997/ , you can open this URL in a browser.

You will see the application listing only when any of the [log-parse-agent](https://github.com/mchopker/log-parse-agent) is running and posting its configuration to this log-parse-ui server.

## Authors

Mahesh Kumar Chopker - mchopker@gmail.com

## Version History

* 1.0.0
    * Initial Release

## Contributing

Pull requests are welcome. For major changes, please open an issue first
to discuss what you would like to change.

Please make sure to update tests as appropriate.

## License

[MIT](https://choosealicense.com/licenses/mit/)

