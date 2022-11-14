package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

type api struct {
	r             *gin.Engine
	storage       Storage
	processor     Processor
	schemaManager SchemaManager
	queueManager  QueueManager
	userManager   UserManager
	Options       *Options
}

type Options struct {
	Port string
}

type Storage interface {
	Ping(ctx context.Context) error
	//	GetList(ctx context.Context, id string) (map[string][]string, error)
}

type Processor interface {
	GenList(ctx context.Context) (string, error)
	AddItem(ctx context.Context, id string, data map[string]string) error
	GetList(ctx context.Context, id string, pageNum, pageSize int) ([]byte, error)
	HandleDelete(id string, data []byte) error
	GetCached(ctx context.Context, name string) (data []byte, err error)
}
type SchemaManager interface {
	GetSchemaJSON(id string) ([]byte, error)
	SaveSchemaJSON(id string, body []byte) error
}

type QueueManager interface {
	PushTask(id string, taskType string, body []byte, name string) error
}

type UserManager interface {
	Check(ctx context.Context, id, token string) error
	CheckWithEmail(ctx context.Context, tokenString string) (string, error)
	NewUser(ctx context.Context, email string) (string, error)
	AddToUserLists(ctx context.Context, id, email string) error
	GetUserIDs(ctx context.Context, email string) (ids []string, err error)
}

func New(st Storage, processor Processor, schemaManager SchemaManager, queueManager QueueManager, userManager UserManager) *api {
	return &api{storage: st,
		processor: processor, schemaManager: schemaManager, queueManager: queueManager, userManager: userManager}
}

func (a *api) Init() {
	a.r = gin.Default()
	corsConfig := cors.DefaultConfig()
	corsConfig.AllowHeaders = append(corsConfig.AllowHeaders, "Token")
	corsConfig.AllowAllOrigins = true
	a.r.Use(cors.New(corsConfig))
	keeper := a.r.Group("/api/list/")
	//	keeper.Use(cors.Default(), a.authenticationMdw)
	a.r.GET("/api/ping", a.ping)
	a.r.GET("/api/pingdb", a.pingDb)
	a.r.POST("/api/list/", a.newList)
	keeper.POST("/:id", a.newItem)
	keeper.GET("/:id/schema", a.getSchema)
	keeper.POST("/:id/schema", a.saveSchema)
	keeper.GET("/:id", a.getList)
	keeper.POST("/:id/bom", a.postBOM)
	keeper.POST("/:id/batch", a.postBatch)
	keeper.GET("/:id/:name", a.getCached)
	a.r.GET("api/user/:email", a.getUserIDs)
	keeper.PUT("/:id", a.deleteItem)
	a.r.POST("/api/login", a.handleLogin)

}

func (a *api) Run() {
	log.Fatal(a.r.Run(":" + a.Options.Port))
}

// Ping: GET state of API
func (a *api) ping(c *gin.Context) {
	c.String(http.StatusOK, "Hello from keeper")
}

// Ping database: GET ping a database
func (a *api) pingDb(c *gin.Context) {
	if err := a.storage.Ping(c); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	c.String(http.StatusOK, "Hello from storage")
}

// New list: POST a new list
func (a *api) newList(c *gin.Context) {
	id, err := a.processor.GenList(c)
	if err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	email, err := a.userManager.CheckWithEmail(c, c.GetHeader("Token"))
	if err != nil {
		c.String(http.StatusUnauthorized, err.Error())
		return
	}
	if err = a.userManager.AddToUserLists(c, id, email); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	c.String(http.StatusCreated, "New list created with ID %s", id)
}

// Get list: get the list by id
func (a *api) getList(c *gin.Context) {
	id := c.Query("id")
	pageNum := c.Query("pageNum")
	pageSize := c.Query("pageSize")
	pageNumInt, err := strconv.Atoi(pageNum)
	if err != nil {
		c.String(http.StatusBadRequest, "incorrect URL arguments ")
		return
	}
	pageSizeInt, err := strconv.Atoi(pageSize)
	if err != nil {
		c.String(http.StatusBadRequest, "incorrect URL arguments ")
		return
	}
	output, err := a.processor.GetList(c, id, pageNumInt, pageSizeInt)
	if err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	c.Header("Content-Type", "application/json")
	c.String(http.StatusFound, string(output))
}

// New Item: POST a new item
func (a *api) newItem(c *gin.Context) {
	id := c.Param("id")
	jsonMap := make(map[string]string)

	defer c.Request.Body.Close()
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	if err = json.Unmarshal(body, &jsonMap); err != nil {
		log.Println(string(body))
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	if err = a.processor.AddItem(c, id, jsonMap); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	c.String(http.StatusCreated, "")
}

// getSchema: GET schema for a list by ID
func (a *api) getSchema(c *gin.Context) {
	id := c.Param("id")

	body, err := a.schemaManager.GetSchemaJSON(id)
	if err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	c.Header("Content-Type", "application/json")
	c.String(http.StatusOK, string(body))
}

func (a *api) saveSchema(c *gin.Context) {
	id := c.Param("id")

	defer c.Request.Body.Close()
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	if err := a.schemaManager.SaveSchemaJSON(id, body); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	c.String(http.StatusOK, "")
}

// postBOM: post components as a BOM
func (a *api) postBOM(c *gin.Context) {
	id := c.Param("id")

	if _, err := a.processor.GetList(c, id, 0, 0); err != nil {
		c.String(http.StatusBadRequest, "no list with such ID")
		return
	}

	if c.Request.Body == nil {
		c.String(http.StatusBadRequest, errors.New("request has no body").Error())
		return
	}
	body, err := c.GetRawData()
	if err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	err = a.queueManager.PushTask(id, "csv", body, id+time.Now().Format(time.RFC3339))
	if err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	c.String(http.StatusAccepted, "")
}

// postBatch: POST components as JSON array
func (a *api) postBatch(c *gin.Context) {
	id := c.Param("id")

	body, err := c.GetRawData()
	if err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	if err = a.queueManager.PushTask(id, "json", body, time.Now().Format(time.RFC3339)); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	c.String(http.StatusAccepted, "")
}

// deleteItem: PUT item out of the list of tracked ones
func (a *api) deleteItem(c *gin.Context) {
	id := c.Param("id")

	body, err := c.GetRawData()
	if err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	if err := a.processor.HandleDelete(id, body); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	c.String(http.StatusAccepted, "")

}

// authenticationMdw: middleware for dealing with API tokens
func (a *api) authenticationMdw(c *gin.Context) {
	listID := c.Param("id")
	token := c.GetHeader("Token")
	if token == "" {
		c.String(http.StatusUnauthorized, "no authentication token found in header Token")
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	err := a.userManager.Check(c, listID, token)
	if err != nil {
		c.String(http.StatusUnauthorized, err.Error())
		err := c.AbortWithError(http.StatusUnauthorized, err)
		log.Println(err.Error())
		return
	}
	c.Next()
}

// handleLogin: handles linking email to a token
func (a *api) handleLogin(c *gin.Context) {
	email, _ := c.GetRawData()
	var token string
	var err error
	if token, err = a.userManager.NewUser(c, string(email)); err != nil {
		log.Println(err.Error(), token)
		c.String(http.StatusUnauthorized, err.Error()+token)
		return
	}
	c.String(http.StatusOK, token)
}

func (a *api) getCached(c *gin.Context) {
	name := c.Param("name")
	var data []byte
	var err error
	if data, err = a.processor.GetCached(c, name); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	c.String(http.StatusOK, string(data))
}

func (a *api) getUserIDs(c *gin.Context) {
	email := c.Param("email")
	ids, err := a.userManager.GetUserIDs(c, email)
	if err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	c.JSON(http.StatusOK, ids)
}
