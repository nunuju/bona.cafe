package db

import (
	"database/sql"
	"fmt"
	"log"

	"meguca/auth"
	"meguca/common"
	"meguca/config"
	"meguca/util"
)

const (
	// TestConnArgs contains ConnArgs used for tests
	TestConnArgs = `user=meguca password=meguca dbname=meguca_test sslmode=disable`
)

var (
	version = len(upgrades) + 1

	noop = func(tx *sql.Tx) (err error) {
		return
	}

	// ConnArgs specifies the PostgreSQL connection arguments
	ConnArgs string

	// IsTest can be overridden to not launch several infinite loops
	// during tests
	IsTest bool

	// Stores the postgres database instance
	db *sql.DB
)

// NOTE(Kagami): DO NOT DELETE ENTRIES! DB version can only grow.
var upgrades = []func(*sql.Tx) error{
	// Old entries.
	noop, noop, noop, noop, noop, noop, noop, noop, noop, noop, noop, noop, noop,
	noop, noop, noop, noop, noop, noop, noop, noop, noop, noop, noop, noop, noop,
	noop, noop, noop, noop, noop, noop, noop, noop, noop, noop, noop, noop, noop,
	noop,
	// Add new post fields.
	func(tx *sql.Tx) (err error) {
		return execAll(tx,
			`DROP FUNCTION insert_thread(id bigint, op bigint, now bigint, board text, auth character varying, body text, ip inet, links bigint[], commands json[], file_cnt bigint, subject character varying)`,
		)
	},
	// Account settings migration.
	func(tx *sql.Tx) (err error) {
		err = execAll(tx,
			`ALTER TABLE accounts
				ADD COLUMN name varchar(20) NOT NULL DEFAULT '',
				ADD COLUMN settings jsonb NOT NULL DEFAULT '{}'`,
		)
		if err != nil {
			return
		}

		// Get accounts.
		var accounts []string
		rs, err := tx.Query("SELECT id FROM accounts")
		if err != nil {
			return
		}
		defer rs.Close()
		for rs.Next() {
			var id string
			if err = rs.Scan(&id); err != nil {
				return
			}
			accounts = append(accounts, id)
		}
		if err = rs.Err(); err != nil {
			return
		}

		// Fill names.
		for _, id := range accounts {
			_, err = tx.Exec("UPDATE accounts SET name = $1 WHERE id = $1", id)
			if err != nil {
				return
			}
		}

		return execAll(tx,
			`ALTER TABLE accounts
				ADD UNIQUE (name),
				ALTER COLUMN name DROP DEFAULT,
				ALTER COLUMN settings DROP DEFAULT`,
		)
	},
	func(tx *sql.Tx) (err error) {
		return execAll(tx,
			`CREATE INDEX sessions_token ON sessions (token)`,
		)
	},
	// Boards settings migration.
	func(tx *sql.Tx) (err error) {
		err = execAll(tx,
			`ALTER TABLE boards
				ADD COLUMN settings jsonb NOT NULL DEFAULT '{}'`,
		)
		if err != nil {
			return
		}

		// Get boards.
		var boards []config.BoardConfig
		rs, err := tx.Query("SELECT id, title, readOnly, modOnly FROM boards")
		if err != nil {
			return
		}
		defer rs.Close()
		for rs.Next() {
			var b config.BoardConfig
			if err = rs.Scan(&b.ID, &b.Title, &b.ReadOnly, &b.ModOnly); err != nil {
				return
			}
			boards = append(boards, b)
		}
		if err = rs.Err(); err != nil {
			return
		}

		// Fill settings.
		for _, b := range boards {
			var settings []byte
			settings, err = b.MarshalJSON()
			if err != nil {
				return
			}
			_, err = tx.Exec("UPDATE boards SET settings = $2 WHERE id = $1", b.ID, settings)
			if err != nil {
				return
			}
		}

		return execAll(tx,
			`ALTER TABLE boards
				DROP COLUMN created,
				DROP COLUMN title,
				DROP COLUMN readOnly,
				ALTER COLUMN settings DROP DEFAULT`,
		)
	},
	func(tx *sql.Tx) (err error) {
		return execAll(tx,
			`INSERT INTO boards
				VALUES ('all', FALSE, '{"title": "Aggregator metaboard"}')`,
		)
	},
	func(tx *sql.Tx) (err error) {
		return execAll(tx,
			`ALTER TABLE staff
				ADD UNIQUE (board, account, position)`,
		)
	},
	func(tx *sql.Tx) (err error) {
		return execAll(tx,
			`CREATE INDEX post_files_post_id ON post_files (post_id)`,
		)
	},
	func(tx *sql.Tx) (err error) {
		return execAll(tx,
			`CREATE INDEX posts_op_time ON posts (op, time)`,
		)
	},
	func(tx *sql.Tx) (err error) {
		return execAll(tx,
			`create table post_reacts (
				smile_name text not null,
				count bigint default 0,
				post_id bigint references posts on delete set null,
				timestamp timestamp default current_timestamp
			  )`,
		)
	},
	func(tx *sql.Tx) (err error) {
		return execAll(tx,
			`create index post_reacts_post_id on post_reacts (post_id)`,
		)
	},
	func(tx *sql.Tx) (err error) {
		return execAll(tx,
			`ALTER TABLE posts ADD COLUMN unique_id text`,
		)
	},
	func(tx *sql.Tx) (err error) {
		return execAll(tx,
			`ALTER TABLE bans ADD COLUMN unique_id text`,
		)
	},
	func(tx *sql.Tx) (err error) {
		return execAll(tx,
			`ALTER TABLE post_reacts ADD COLUMN id bigserial primary key`,
		)
	},
	func(tx *sql.Tx) (err error) {
		return execAll(tx,
			`CREATE TABLE user_reacts (
				account_id varchar(20) references accounts on delete cascade,
				ip inet,
				post_react_id bigserial references post_reacts on delete cascade
			)`,
		)
	},
	func(tx *sql.Tx) (err error) {
		return execAll(tx, `
			ALTER TABLE post_reacts ADD FOREIGN KEY (post_id) REFERENCES posts on delete cascade`,
		)
	},
	func(tx *sql.Tx) (err error) {
		return execAll(tx, `
			CREATE INDEX user_reacts_post_react_id ON user_reacts (post_react_id)`,
		)
	},
	func(tx *sql.Tx) (err error) {
		return execAll(tx, `
			CREATE TABLE smiles (
				name text not null,
				aliases text[],
				board text not null references boards on delete cascade,
				fileType smallint not null,
				id bigint primary key,
				deleted boolean,
				deleted_at timestamp,
				created timestamp default (now() at time zone 'utc')
			)`,
		)
	},
	func(tx *sql.Tx) (err error) {
		return execAll(tx, `
			CREATE INDEX smiles_name ON smiles (name);
		`)
	},
	func(tx *sql.Tx) (err error) {
		return execAll(tx, `
			CREATE INDEX smiles_id ON smiles (id);
		`)
	},
	func(tx *sql.Tx) (err error) {
		return execAll(tx, `
			ALTER TABLE smiles ADD COLUMN file_hash char(40) not null`,
		)
	},
	func(tx *sql.Tx) (err error) {
		return execAll(tx, `
			ALTER TABLE smiles DROP COLUMN id, ADD COLUMN id bigserial primary key;`,
		)
	},
	func(tx *sql.Tx) (err error) {
		return execAll(tx, `
			ALTER TABLE smiles DROP COLUMN fileType, ADD COLUMN file_type smallint not null;`,
		)
	},
	// Remove all existing reactions to recreate model.
	// There is a couple solutions to preserve data,
	// but it's not that big of a deal.
	func(tx *sql.Tx) (err error) {
		return execAll(tx, `
			DELETE from post_reacts;
			ALTER TABLE post_reacts DROP COLUMN IF EXISTS smile_name;
			ALTER TABLE post_reacts ADD COLUMN smile_id bigserial references smiles on delete cascade;
			create index post_reacts_smile_id on post_reacts (smile_id);`,
		)
	},
	func(tx *sql.Tx) (err error) {
		return execAll(tx, `
			ALTER TABLE smiles ADD COLUMN readonly boolean default false;`,
		)
	},
}

func StartDB() (err error) {
	if db, err = sql.Open("postgres", ConnArgs); err != nil {
		return
	}

	var exists bool
	err = db.QueryRow(getQuery("init/check_db_exists.sql")).Scan(&exists)
	if err != nil {
		return
	}

	tasks := []func() error{}
	if exists {
		tasks = append(tasks, upgradeDB)
	} else {
		tasks = append(tasks, initDB)
	}
	/* tasks = append(tasks, startKpopnetDB) */
	tasks = append(tasks, genPrepared)
	if !exists {
		tasks = append(tasks, createAdminAccount)
	}
	tasks = append(tasks, loadServerConfig, loadBoardConfigs, loadBans)
	if err = util.Waterfall(tasks...); err != nil {
		return
	}

	go runCleanupTasks()
	return
}

func initDB() error {
	log.Println("initializing database")

	conf, err := config.DefaultServerConfig.MarshalJSON()
	if err != nil {
		return err
	}

	q := fmt.Sprintf(getQuery("init/init.sql"), version, string(conf))
	_, err = db.Exec(q)
	return err
}

// Check database version perform any upgrades.
func upgradeDB() (err error) {
	var v int
	err = db.QueryRow(`select val from main where id = 'version'`).Scan(&v)
	if err != nil {
		return
	}

	var tx *sql.Tx
	for i := v; i < version; i++ {
		log.Printf("upgrading database to version %d\n", i+1)
		tx, err = db.Begin()
		if err != nil {
			return
		}

		err = upgrades[i-1](tx)
		if err != nil {
			return rollBack(tx, err)
		}

		// Write new version number
		_, err = tx.Exec(
			`update main set val = $1 where id = 'version'`,
			i+1,
		)
		if err != nil {
			return rollBack(tx, err)
		}

		err = tx.Commit()
		if err != nil {
			return
		}
	}

	return
}

func rollBack(tx *sql.Tx, err error) error {
	if rbErr := tx.Rollback(); rbErr != nil {
		err = util.WrapError(err.Error(), rbErr)
	}
	return err
}

/* func startKpopnetDB() (err error) {
	return kpopnet.StartDb(db, ConnArgs)
} */

// Create admin account with default password.
func createAdminAccount() error {
	hash, err := auth.BcryptHash(common.DefaultAdminPassword, 10)
	if err != nil {
		return err
	}
	return RegisterAccount("admin", hash)
}
