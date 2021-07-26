# onlineconf-myteam-bot

`onlineconf-myteam-bot` is used to send [OnlineConf](https://github.com/onlineconf/onlineconf) configuration changes notifications to subscribed users in the [Myteam](https://biz.mail.ru/myteam/) messenger.

## Requirements

OnlineConf (`onlineconf-admin`) >= v3.5.0

## Installation

`onlineconf-myteam-bot` uses MySQL database to store user subscriptions and intermediate information.
The database must be created and then populated with tables using [schema.sql](/schema.sql).
An address and credentials of the database must be stored in the `/database` parameters (see below).

The only one instance of `onlineconf-myteam-bot` daemon should be run simultaneously.

## Configuration

`onlineconf-myteam-bot` uses OnlineConf BotAPI to retrieve notifications from OnlineConf.
To add BotAPI user for `onlineconf-myteam-bot` the following configuration is required:

* `/onlineconf/botapi/bot/onlineconf-myteam-bot` - the parameter name is the username, a value is SHA256 of a password (required)
* `/onlineconf/botapi/bot/onlineconf-myteam-bot/scopes` = `notifications` (list, required)

`onlineconf-myteam-bot` is configured using OnlineConf itself, it reads `onlineconf-myteam-bot` module (`/usr/local/etc/onlineconf-myteam-bot.cdb` file).
The module must be configured to contain the following `/`-separated parameters:

* `database`
	* `base` - database name (default: `onlineconf_myteam_bot`)
	* `host` - database host (required)
	* `pass` - database password (required)
	* `user` - database username (default: `onlineconf_myteam_bot`)
* `myteam`
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
	* `domain` - domain name appended to OnlineConf username to make it Myteam username
	* `map` - YAML/JSON-mapping of non-standard usernames from OnlineConf to Myteam (without domain)

This configuration must be placed in OnlineConf under `/onlineconf/module/onlineconf-myteam-bot`.

The best way to achieve the described configuration on a server (or a container) where only `onlineconf-myteam-bot` is run is to create the following parameters:

| Path | Type | Value |
| ---- | ---- | ----- |
| `/onlineconf/botapi/bot/onlineconf-myteam-bot` | Text | SHA256 of a password used by `onlineconf-myteam-bot` to connect to OnlineConf BotAPI |
| `/onlineconf/botapi/bot/onlineconf-myteam-bot/scopes` | List | `notifications` |
| `/onlineconf/service/onlineconf-myteam-bot` | Text | SHA256 of a password used by `onlineconf-updater`/`onlineconf-csi-driver` |
| `/onlineconf/module` | Case | key: service = `onlineconf-myteam-bot`<br/>value: symlink to `/onlineconf/chroot/onlineconf-myteam-bot` |
| `/onlineconf/chroot/onlineconf-myteam-bot` | YAML | `delimiter: /` |
| `/onlineconf/chroot/onlineconf-myteam-bot/onlineconf-myteam-bot` | Symlink | `/onlineconf/myteam-bot` |
| `/onlineconf/myteam-bot` | Null | value is Null, children must contain the module structure described above |
