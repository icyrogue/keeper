package requestprocessor

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"math/rand"
	"time"

	"github.com/icyrogue/ye-keeper/internal/jsonmodels"
)

type Storage interface {
	NewList(ctx context.Context, id string) (bool, error)
	AddItem(ctx context.Context, args [][]interface{}) error
	GetList(ctx context.Context, id string, pageNum, pageSize int) ([]byte, error)
}

type SchemaManager interface {
	CompareFieldNames(id string, data []string) ([]string, error)
	GetNames(id string) ([]string, int, error)
	GetParams(id string) (map[string]string, error)
}

type MultiEncoder interface {
	EncodeJSON(component, params map[string]string) ([]byte, error)
}

type Analyzer interface {
	GetInput() chan []string
	GetDeleteInput() chan []byte
}

type CacheManager interface {
	Get(ctx context.Context, name string) chan []jsonmodels.JSONResponse
}

type requestProcessor struct {
	st            Storage
	SchemaManager SchemaManager
	multiEncoder  MultiEncoder
	analyzer      Analyzer
	cacheManager  CacheManager
}

func New(st Storage, schemaManager SchemaManager, multiEncoder MultiEncoder, analyzer Analyzer, cacheManager CacheManager) *requestProcessor {
	return &requestProcessor{st: st, SchemaManager: schemaManager,
		multiEncoder: multiEncoder, analyzer: analyzer, cacheManager: cacheManager}
}

// Get a seed so that ids are random every time
func init() {
	rand.Seed(time.Now().UnixMicro())
}

// GenID: generates a new ID until there is no such ID already in database
func genID() string {
	chars := []byte("qwertyuiopasdfghklzxcvbnmQWERTYUIOPASDFGHJKLZXCVBNM")
	output := []byte{}
	for i := 0; i != 8; i++ {
		output = append(output, chars[rand.Intn(len(chars))])
	}
	return string(output)
}

// Gen list: generates new list and returns its ID
func (p *requestProcessor) GenList(ctx context.Context) (string, error) {
	id := genID()
	copy, err := p.st.NewList(ctx, id)
	if err != nil {
		for copy {
			copy, err = p.st.NewList(ctx, genID())
		}
		if err != nil {
			return "", err
		}
	}
	return id, nil
}

// Add item: adds single item to db
func (p *requestProcessor) AddItem(ctx context.Context, id string, data map[string]string) error {
	var names []string
	args := make([]interface{}, 4)
	for key := range data {
		names = append(names, key)
	}
	_, err := p.SchemaManager.CompareFieldNames(id, names)
	if err != nil {
		return err
	}
	params, err := p.SchemaManager.GetParams(id)
	if err != nil {
		return err
	}
	body, err := p.multiEncoder.EncodeJSON(data, params)
	if err != nil {
		return err
	}
	args[0] = id                //id column id db
	args[1] = data["part name"] //component name
	args[2] = body              //component record itself along with params
	args[3] = true              //tracking: TRUE means that component should be tracked
	log.Println("adding items to storage")
	err = p.st.AddItem(ctx, [][]interface{}{args})
	return err
}

// Get list: returns list as JSON array of components
func (p *requestProcessor) GetList(ctx context.Context, id string, pageNum, pageSize int) ([]byte, error) {
	log.Println("got req for a list", id, pageNum, pageSize)
	data, err := p.st.GetList(ctx, id, pageNum, pageSize)
	if err != nil {
		log.Println(err.Error())
		return nil, err
	}
	log.Println(string(data))
	if len(data) == 0 {
		return nil, nil
	}
	log.Println(string(data))
	return data, nil
}

// HandleDelete: passes delete req to worker
func (p *requestProcessor) HandleDelete(id string, data []byte) error {
	p.analyzer.GetDeleteInput() <- append(data, []byte(id)...)
	return nil
}

func (p *requestProcessor) GetCached(ctx context.Context, name string) (data []byte, err error) {
	v, ok := <-p.cacheManager.Get(ctx, name)
	if !ok {
		return nil, errors.New("couldn't get information about component " + name + " after 10 tries")
	}
	if data, err = json.Marshal(&v); err != nil {
		return nil, err
	}
	return data, nil
}
