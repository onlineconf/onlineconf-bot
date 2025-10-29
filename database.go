package onlineconfbot

import (
	"context"
	"database/sql"
	"net"
	"strings"

	"github.com/go-sql-driver/mysql"
)

type SubscriptionStorage interface {
	Subscribe(context.Context, string, bool) error
	Unsubscribe(context.Context, string) error
	Subscribers(context.Context) ([]Subscription, error)
}

type database struct {
	*sql.DB
}

type transaction struct {
	*sql.Tx
}

func openDatabase() (*database, error) {
	defaultName := strings.ReplaceAll(commandName, "-", "_")
	mysqlConfig := mysql.NewConfig()
	mysqlConfig.User = config.GetString("/database/user", defaultName)
	mysqlConfig.Passwd = config.GetString("/database/pass", "")
	mysqlConfig.Net = "tcp"
	mysqlConfig.Addr = net.JoinHostPort(config.GetString("/database/host", ""), config.GetString("/database/port", "3306"))
	mysqlConfig.DBName = config.GetString("/database/base", defaultName)
	mysqlConfig.Params = map[string]string{
		"charset":   "utf8mb4",
		"collation": "utf8mb4_general_ci",
	}

	db, err := sql.Open("mysql", mysqlConfig.FormatDSN())
	if err != nil {
		return nil, err
	}

	return &database{db}, nil
}

func (db *database) BeginTx(ctx context.Context) (*transaction, error) {
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &transaction{tx}, nil
}

func (tx *transaction) GetLastID(ctx context.Context, botID int) (int, error) {
	row := tx.QueryRowContext(ctx, "SELECT Value FROM lastid WHERE ID = ? FOR UPDATE", botID)
	var lastID int
	err := row.Scan(&lastID)
	if err != nil {
		return 0, err
	}
	return lastID, nil
}

func (tx *transaction) SetLastID(ctx context.Context, lastID int, botID int) error {
	_, err := tx.ExecContext(ctx, "UPDATE lastid SET Value = ? WHERE ID = ?", lastID, botID)
	return err
}

func (db *database) Subscribe(ctx context.Context, user string, wo bool) error {
	_, err := db.ExecContext(ctx, "INSERT INTO subscribe (User, WO) VALUES (?, ?) ON DUPLICATE KEY UPDATE WO=VALUES(WO)", user, wo)
	return err
}

func (db *database) Unsubscribe(ctx context.Context, user string) error {
	_, err := db.ExecContext(ctx, "DELETE FROM subscribe WHERE User = ?", user)
	return err
}

type Subscription struct {
	User string
	WO   bool
}

func (db *database) Subscribers(ctx context.Context) ([]Subscription, error) {
	rows, err := db.QueryContext(ctx, "SELECT User, WO FROM subscribe ORDER BY User")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	subscriptions := []Subscription{}
	for rows.Next() {
		var subscription Subscription
		err := rows.Scan(&subscription.User, &subscription.WO)
		if err != nil {
			return nil, err
		}
		subscriptions = append(subscriptions, subscription)
	}
	return subscriptions, nil
}

func (db *database) FilterSubscribed(ctx context.Context, users map[string]string) ([]string, error) {
	write := []string{}
	read := []string{}
	for user, access := range users {
		switch access {
		case "rw":
			write = append(write, user)
		case "ro":
			read = append(read, user)
		}
	}
	query := strings.Builder{}
	query.WriteString("SELECT User FROM subscribe WHERE ")
	bind := []interface{}{}
	if len(write) > 0 {
		query.WriteString("User IN (")
		for i, user := range write {
			query.WriteString("?")
			if i+1 != len(write) {
				query.WriteString(", ")
			}
			bind = append(bind, user)
		}
		query.WriteString(")")
	}
	if len(read) > 0 {
		if len(bind) > 0 {
			query.WriteString(" OR ")
		}
		query.WriteString("(User IN (")
		for i, user := range read {
			query.WriteString("?")
			if i+1 != len(read) {
				query.WriteString(", ")
			}
			bind = append(bind, user)
		}
		query.WriteString(") AND NOT WO)")
	}
	if query.Len() == 0 {
		return []string{}, nil
	}
	rows, err := db.QueryContext(ctx, query.String(), bind...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	notifyUsers := []string{}
	for rows.Next() {
		var user string
		err := rows.Scan(&user)
		if err != nil {
			return nil, err
		}
		notifyUsers = append(notifyUsers, user)
	}
	return notifyUsers, nil
}
