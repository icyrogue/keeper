package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/icyrogue/ye-keeper/internal/jsonmodels"
)

type jsonResp struct {
	jsonmodels.JSONResponse
}
type client struct {
	router              *resty.Client
	components          []component
	schemaManager       SchemaManager
	queueManager        QueueManager
	notificationManager NotificationManager
	cacheManager        CacheManager
	storage             Storage
	mailTemplate        []byte
	Options             *Options
}

type component struct {
	id        string
	minAmount int
	region    string
	name      string
	priority  bool
}

type Options struct {
	MaxRequestsPer int
	MaxTimeOutTime int
	APIToken       string
	MailTempPath   string
}

type jsonError struct {
	Error string `json:"error"`
}

type QueueManager interface {
	PushTask(id string, taskType string, body []byte, name string) error
}

type NotificationManager interface {
	Notify(ctx context.Context, id string, data []byte) error
}

type SchemaManager interface {
	GetParams(id string) (map[string]string, error)
	GetAllIDs() []string
}

type Storage interface {
	GetComponents(ctx context.Context) ([]string, []string, error)
}

type CacheManager interface {
	Check(name string) (cached bool)
	Get(ctx context.Context, name string) chan []jsonmodels.JSONResponse
	Store(name string, data []jsonmodels.JSONResponse)
}

const (
	StatusBandWidthLimitExceeded = 509 //EFind "too many requests" code
	apiURL                       = "https://efind.ru/api/search"
)

func New(schemaManager SchemaManager, storage Storage, queueManager QueueManager, notificationManager NotificationManager, cacheMnager CacheManager) *client {
	return &client{router: resty.New(), schemaManager: schemaManager, storage: storage,
		queueManager: queueManager, notificationManager: notificationManager, cacheManager: cacheMnager}
}

func (c *client) Start(ctx context.Context) {
	ticker := time.NewTicker(time.Second * time.Duration(c.Options.MaxTimeOutTime))
	var pop component
	var err error
	c.mailTemplate, err = os.ReadFile(c.Options.MailTempPath)
	if err != nil {
		log.Println(err.Error())
		return
	}
	c.Update()
	go func() {
	loop:
		for {
			select {
			case <-ctx.Done():
				break loop
			case <-ticker.C:
				for i := 0; i < c.Options.MaxRequestsPer; i++ {
					cut := len(c.components) - 1
					if cut <= 0 {
						break
					}
					pop = c.components[cut:][0]
					c.components = c.components[:cut]
					if !c.cacheManager.Check(pop.name) {
						go c.check(pop)
						continue loop
					}
					go c.getFromCache(pop)
				}
				c.Update()
			}
		}
	}()
}

// Update: pushes new components to clients array of components to check
func (c *client) Update() {
	data, ids, err := c.storage.GetComponents(context.Background())
	if len(data) == 0 {
		log.Println(data)
		return
	}
	if err != nil {
		log.Println(err.Error())
		return
	}
	log.Println(data, ids)
	for i, id := range ids {
		comp := component{}

		schema, err := c.schemaManager.GetParams(id)
		if err != nil {
			log.Println("client got:", err.Error(), id)
			continue
		}
		minAmount, err := strconv.Atoi(schema["minimumAmount"])
		if err != nil {
			log.Println(err.Error())
			continue
		}

		comp.id = id
		comp.minAmount = minAmount

		var fd bool
		comp.region, fd = schema["region"]

		if !fd {
			log.Println(err.Error())
			continue
		}

		comp.name = data[i]
		c.components = append(c.components, comp)
	}
	log.Println("components left to check:", len(c.components))
}

// check: checks availability of components it preferred region
// if it isnt available checks again for alternatives and informs a user
func (c *client) check(comp component) {
	if comp.id == "" {
		return
	}
	log.Println("checking for", comp.id)

	resp, err := c.router.R().SetQueryParam("access_token", c.Options.APIToken).SetQueryParam("r", comp.region).SetQueryParam("stock", "1").Get(apiURL + "/" + comp.name)
	if err != nil {
		log.Println(err.Error())
		return
	}

	if resp.IsError() {
		log.Println("client got an error")
		//if client exeeded req limit - push component back into queue
		if resp.StatusCode() == StatusBandWidthLimitExceeded {
			log.Println("client exceeded req limits")
			comp.priority = true
		}
		var jsonErr jsonError
		if err = json.Unmarshal(resp.Body(), &jsonErr); err != nil {
			log.Println(err)
			return
		}
		log.Println(jsonErr.Error)
		return
	}

	var respJSON []jsonmodels.JSONResponse
	err = json.Unmarshal(resp.Body(), &respJSON)
	if err != nil {
		log.Println(err.Error())
	}

	c.cacheManager.Store(comp.name, respJSON)
	if err != nil {
		log.Println(err.Error())
	}

	err = c.handleResponse(respJSON, comp)
	if err != nil {
		log.Println(err.Error())
	}
}

func (c *client) handleResponse(data []jsonmodels.JSONResponse, comp component) error {
	ln := len(data)
	respJSON := make([]jsonResp, ln)
	if ln != 0 {
		for i, rs := range data {
			respJSON[i] = jsonResp{rs}
		}
		return nil
	}
	body, err := c.construct(comp.id, comp.name, respJSON)
	if err != nil {
		return err
	}
	if err := c.notificationManager.Notify(context.Background(), comp.id, body); err != nil {
		return err
	}
	return nil
}

func (c *client) getFromCache(component component) {
	data, ok := <-c.cacheManager.Get(context.Background(), component.name)
	if !ok {
		log.Println("couldn't get component from cache")
		return
	}
	if err := c.handleResponse(data, component); err != nil {
		log.Print(err.Error())
		return
	}
}

// construct: creates email from template and response
func (c *client) construct(id, name string, data []jsonResp) ([]byte, error) {
	template := c.mailTemplate
	if template == nil {
		return nil, nil
	}
	log.Println("email construction for", id)

	r := regexp.MustCompile("{name}")
	template = r.ReplaceAll(template, []byte(name))

	r = regexp.MustCompile("{id}")
	template = r.ReplaceAll(template, []byte(id))

	r = regexp.MustCompile("{time}")
	template = r.ReplaceAll(template, []byte(time.Now().Format(time.RFC3339)))

	var table bytes.Buffer

	if len(data) == 0 {
		table.WriteString("Других доступных варинтов нет, попробуйте изменить мин. количество")
	} else {
		for _, alt := range data {
			table.WriteString(alt.toString())
		}
	}

	r = regexp.MustCompile("{alt}")
	template = r.ReplaceAll(template, table.Bytes())

	return template, nil
}

// getParentRegion: returns new region to check for alternatives
func (cm *component) getParentRegion() string {
	return "1"
}

// toString: converts row from response to alternative for email HTML
func (r *jsonResp) toString() string {
	var output strings.Builder
	header := `<h3><a href="%s">%s</a> %s</h3>
	<details>
    <summary>Больше информации: </summary>
    <span>%s</span>
	</details>
	<span>%s</span>
	<br>`
	//TODO: add phones
	header = fmt.Sprintf(header, r.Stockdata.Site, r.Stockdata.Title,
		r.Stockdata.City, r.Stockdata.Email, r.Stockdata.Limits)
	output.WriteString(header)
	//TODO: add price sorting
	row := `<table border="1" cellspacing="0" cellpadding="0" width="200" align="center"><tr>
	<th>Название</th>
	<th>Производитель</th>
	<th>Наличие</th>
	<th>Цена</th>
	</tr>`
	output.WriteString(row)
	row = `<tr>
	<td><a href="%s">%s</a></td>
	<td>%s</td>
	<td>%s</td>
	<td>%s</td>
	</tr>`
	for _, el := range r.Rows {
		var prices string
		for _, pr := range el.Price {
			prices = prices + fmt.Sprintf("\n%v р./шт от %v", pr[1], pr[0])
		}
		output.WriteString(fmt.Sprintf(row, el.URL, el.Name,
			el.Manufacturer, el.Stock, prices))
	}
	output.WriteString("</table>")
	return output.String()
}
