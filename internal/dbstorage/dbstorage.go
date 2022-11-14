package dbstorage

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type storage struct {
	db      *pgxpool.Pool
	Options *Options
}

type Options struct {
	Dsn string
}

func New() *storage {
	return &storage{}
}

func (st *storage) Init() error {
	conn, err := pgxpool.New(context.Background(), st.Options.Dsn)
	if err != nil {
		return err
	}
	log.Println("connecting to ", st.Options.Dsn)
	st.db = conn
	_, err = st.db.Exec(context.Background(), `CREATE table IF NOT EXISTS "components" (id TEXT, name TEXT, schema JSONB, tracking BOOL, lastcheck TIMESTAMP)`)
	if err != nil {
		return err
	}
	log.Println("CONNECTED")
	return nil
}

func (st *storage) Close() {
	st.db.Close()
}

// Ping: pings db! Never pongs tho
func (st *storage) Ping(ctx context.Context) error {
	return st.db.Ping(ctx)

}

// NewList: searches for list ID in db, if it already exists returns true
func (st *storage) NewList(ctx context.Context, id string) (bool, error) {
	_, err := st.db.Exec(ctx, `SELECT * FROM "components" WHERE id = $1`, id)
	if err != nil {
		return false, err
	}
	return true, nil
}

// Converts: all components from list to JSON array
func (st *storage) GetList(ctx context.Context, id string, pageNum, pageSize int) ([]byte, error) {
	rows, err := st.db.Query(ctx, `SELECT json_build_object('pageNumber', $3::int, 'pageSize', $1::int, 'components', (SELECT json_agg(c) FROM (SELECT comp AS component from(SELECT distinct on (schema->'component') schema->'component' as comp, tracking from components
ORDER by schema->'component', ctid DESC) as cmps WHERE cmps.tracking = true OFFSET $2 FETCH NEXT $1 row ONLY)
 c));`, pageSize, pageSize*(pageNum-1), pageNum)
	if err != nil {
		return nil, err
	}
	var body []byte
	for rows.Next() {
		err = rows.Scan(&body)
		if err != nil {
			return nil, err
		}
	}
	log.Println(string(body))
	return body, nil
}

// AddItem: adds item from list of arguments for columns
func (st *storage) AddItem(ctx context.Context, args [][]interface{}) error {
	names := []string{"id", "name", "schema", "tracking"}
	_, err := st.db.CopyFrom(ctx, pgx.Identifier{"components"}, names, pgx.CopyFromRows(args))
	if err != nil {
		return err
	}
	return nil
}

// GetComponents: returns a batch of components with IDs for client to check
// updates timestamp when component was last checked so that next batch has
// components that werent checked or were checked long time ago
func (st *storage) GetComponents(ctx context.Context) ([]string, []string, error) {
	var output []string
	var ids []string
	rows, err := st.db.Query(ctx, `UPDATE components SET lastcheck = NOW()
WHERE schema = ANY (SELECT foo.schema FROM (SELECT DISTINCT ON (schema) * FROM components
ORDER BY schema, ctid DESC)
as foo WHERE foo.tracking = true ORDER BY foo.lastcheck  FETCH NEXT 9 ROWS ONLY) RETURNING id, name`)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var data string
		var id string
		err := rows.Scan(&id, &data)
		if err != nil {
			return nil, nil, err
		}
		output = append(output, data)
		ids = append(ids, id)
	}
	if rows.Err() != nil {
		return nil, nil, err
	}
	return output, ids, nil
}

// SyncSchemas: returns every schema from db for schemaManager
func (st *storage) SyncSchemas(ctx context.Context) ([]byte, error) {
	data, err := st.db.Query(ctx, `SELECT json_agg(schema->'parameters') from "components"`)
	if err != nil {
		return nil, err
	}
	var schema []byte
	for data.Next() {
		err := data.Scan(&schema)
		if err != nil {
			return nil, err
		}
	}
	return schema, nil
}

// GetPool: returns conn, needed for userManager
func (st *storage) GetPool() *pgxpool.Pool {
	return st.db
}
