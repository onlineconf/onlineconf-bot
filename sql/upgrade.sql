ALTER TABLE myteam_lastid RENAME TO lastid;
ALTER TABLE myteam_subscribe RENAME TO subscribe;

-- the audit feed cursor (onlineconf-admin with value encryption)
INSERT IGNORE INTO `lastid` VALUES (1000, 0);
