CREATE TABLE `myteam_lastid` (
	`ID` int(11) NOT NULL DEFAULT '0',
	`Value` bigint(20) unsigned NOT NULL DEFAULT '0',
	PRIMARY KEY (`ID`)
);

INSERT INTO `myteam_lastid` VALUES (0, 0);

CREATE TABLE `myteam_subscribe` (
	`User` varchar(128) NOT NULL,
	`WO` tinyint(1) NOT NULL DEFAULT '1',
	PRIMARY KEY (`User`)
)
