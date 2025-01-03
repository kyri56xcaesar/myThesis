package userspace

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

/* Structure containing all needed aspects of this service */
type UService struct {
	/* operator logic of the service
	*   -> perhaps this should be attachable <-
	* */
	/* server engine */
	Engine *gin.Engine

	/* database calls functions  */
	dbh DBHandler

	/* configuration file (.env) */
	config EnvConfig
}

/* "constructor" */
func NewUService(conf string) UService {
	cfg := LoadConfig(conf)

	srv := UService{
		Engine: gin.Default(),
		config: cfg,
		dbh: DBHandler{
			DBName: cfg.DB,
		},
	}
	srv.dbh.Init()

	return srv
}

/* listen on http at handled endpoints */
func (srv *UService) Serve() {
	root := srv.Engine.Group("/")
	{
		root.GET("/healthz", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"message": "alive",
			})
		})
	}
	/* These endpoints should parse an authentication token and handle verification of authorization according
	*    to the permissions of the user. For now, we will just implement the endpoints without any
	* */
	apiV1 := srv.Engine.Group("/api/v1")
	{
		/* equivalent to "ls", will return the resources, from the given path*/
		apiV1.GET("/resources", srv.ListResources)
		apiV1.POST("/resources", srv.PostResources)
		apiV1.PUT("/resources", srv.MoveResources)

		apiV1.GET("/resource/download", srv.DownloadResource)
	}

	admin := srv.Engine.Group("/admin")
	{
		admin.PATCH("/resource/permissions", srv.ChmodResource)
		admin.PATCH("/resource/ownership", srv.ChownResource)
	}

	/* context handler */
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	/* server std lib raw definition */
	server := &http.Server{
		Addr:              srv.config.Addr(),
		Handler:           srv.Engine,
		ReadHeaderTimeout: time.Second * 5,
	}

	/* listen in a goroutine */
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	/* handle signals on process ..
	*
	* */
	<-ctx.Done()

	stop()
	log.Println("shutting down gracefully, press Ctrl+C again to force")

	/* don't forgt to close the db conn */
	srv.dbh.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown: ", err)
	}

	log.Println("Server exiting")
}
