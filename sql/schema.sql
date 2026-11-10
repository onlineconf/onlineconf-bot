CREATE TABLE `lastid` (
	`ID` int(11) NOT NULL DEFAULT '0',
	`Value` bigint(20) unsigned NOT NULL DEFAULT '0',
	PRIMARY KEY (`ID`)
);

-- one cursor per BotAPI feed: 0 = change notifications, 1000 = audit events
-- (the ids in between are left to per-bot notification cursors)
INSERT INTO `lastid` VALUES (0, 0);
INSERT INTO `lastid` VALUES (1000, 0);

CREATE TABLE `subscribe` (
	`User` varchar(128) NOT NULL,
	`WO` tinyint(1) NOT NULL DEFAULT '1',
	PRIMARY KEY (`User`)
);
