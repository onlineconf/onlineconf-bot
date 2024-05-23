# onlineconf-bot

`onlineconf-bot` is used to send [OnlineConf](https://github.com/onlineconf/onlineconf) configuration changes notifications to subscribed users using [Myteam](https://biz.mail.ru/myteam/) or [Mattermost](https://mattermost.com/) messengers.

## Requirements

OnlineConf (`onlineconf-admin`) >= v3.5.0

## Installation

`onlineconf-bot` uses MySQL database to store user subscriptions and intermediate information.
The database must be created and then populated with tables using [schema.sql](/schema.sql).
An address and credentials of the database must be stored in the `/database` parameters (see below).

The only one instance of `onlineconf-bot` daemon should be run simultaneously.

Two separate binaries will be built for each messenger supported, see `Dockerfile` for an example.

## Configuration

`onlineconf-bot` uses OnlineConf BotAPI to retrieve notifications from OnlineConf.
To add BotAPI user for `onlineconf-bot` the following configuration is required:

* `/onlineconf/botapi/bot/onlineconf-bot` - the parameter name is the username, a value is SHA256 of a password (required)
* `/onlineconf/botapi/bot/onlineconf-bot/scopes` = `notifications` (list, required)

`onlineconf-bot` is configured using OnlineConf itself, it reads `onlineconf-bot` module (`/usr/local/etc/onlineconf-bot.cdb` file).
The module must be configured to contain the following `/`-separated parameters:

* `database`
	* `base` - database name (default: `onlineconf_bot`)
	* `host` - database host (required)
	* `pass` - database password (required)
	* `user` - database username (default: `onlineconf_bot`)
* `mattermost` (only used by `onlineconf-mattermost-bot`)
    * `api-url` - Mattermost API base URL (i.e. scheme and hostname)
    * `ws-url` - Mattermost Websocket base URL
    * `token` - Mattermost bot token
* `myteam` (only used by `onlineconf-myteam-bot`)
	* `token` - a bot token retrieved from Metabot (required)
	* `url` - URL of an alternative Myteam installation
* `onlineconf`
	* `botapi`
		* `password` - password (required)
		* `url` - URL of OnlineConf BotAPI (required)
		* `username` - username (default: `onlineconf-myteam-bot`)
		* `wait` - long polling wait time (default: `60`)
	* `link-url` - URL of OnlineConf UI (required)
* `user`
	* `domain` - domain name appended to OnlineConf username to match the messenger account
	* `map` - YAML/JSON-mapping of non-standard usernames from OnlineConf to the messenger account (without domain name)

This configuration must be placed in OnlineConf under `/onlineconf/module/onlineconf-bot`.

The best way to achieve the described configuration on a server (or a container) where only `onlineconf-bot` is run is to create the following parameters:

| Path | Type | Value |
| ---- | ---- | ----- |
| `/onlineconf/botapi/bot/onlineconf-bot` | Text | SHA256 of a password used by `onlineconf-bot` to connect to OnlineConf BotAPI |
| `/onlineconf/botapi/bot/onlineconf-bot/scopes` | List | `notifications` |
| `/onlineconf/service/onlineconf-bot` | Text | SHA256 of a password used by `onlineconf-updater`/`onlineconf-csi-driver` |
| `/onlineconf/module` | Case | key: service = `onlineconf-bot`<br/>value: symlink to `/onlineconf/chroot/onlineconf-bot` |
| `/onlineconf/chroot/onlineconf-bot` | YAML | `delimiter: /` |
| `/onlineconf/chroot/onlineconf-bot/onlineconf-bot` | Symlink | `/onlineconf/bot` |
| `/onlineconf/bot` | Null | value is Null, children must contain the module structure described above |
