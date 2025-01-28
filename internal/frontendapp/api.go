package frontendapp

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os/signal"
	"reflect"
	"syscall"
	"time"

	ut "kyri56xcaesar/myThesis/internal/utils"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
)

/*
*
* Constants */
const (
	apiVersion    string = "v1"
	templatesPath string = "./web/auther/templates"
	staticsPath   string = "web/auther/static"
)

/*
*
* Structs */
type HTTPService struct {
	Engine *gin.Engine
	Config ut.EnvConfig
}

/*
*
* Structs */
func NewService(conf string) HTTPService {
	service := HTTPService{}
	service.Engine = gin.Default()
	service.Config = ut.LoadConfig(conf)

	authServiceURL = fmt.Sprintf("http://%s:%s", service.Config.IP, service.Config.AUTH_PORT)
	apiServiceURL = fmt.Sprintf("http://%s:%s", service.Config.IP, service.Config.API_PORT)

	return service
}

// Serve function
func (srv *HTTPService) ServeHTTP() {
	corsconfig := cors.DefaultConfig()
	corsconfig.AllowOrigins = srv.Config.AllowedOrigins
	corsconfig.AllowMethods = srv.Config.AllowedMethods
	corsconfig.AllowHeaders = srv.Config.AllowedHeaders

	// template functions
	funcMap := template.FuncMap{
		"sub":    func(a, b int) int { return a - b },
		"typeIs": func(value interface{}, t string) bool { return reflect.TypeOf(value).Kind().String() == t },
		"hasKey": func(value map[string]interface{}, key string) bool {
			_, exists := value[key]
			return exists
		},
	}

	// set a template eng
	tmpl := template.Must(template.New("").Funcs(funcMap).ParseGlob(templatesPath + "/*.html"))

	// Api
	srv.Engine.SetHTMLTemplate(tmpl)
	srv.Engine.Use(static.Serve("/api/"+apiVersion, static.LocalFile(staticsPath, true)))
	srv.Engine.Use(cors.New(corsconfig))

	root := srv.Engine.Group("/")
	{
		root.GET("/", func(c *gin.Context) {
			c.Redirect(http.StatusFound, "/api/"+apiVersion)
		})

		root.GET("/healthz", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"status": "alive",
			})
		})
	}

	apiV1 := srv.Engine.Group("/api/" + apiVersion)
	{
		apiV1.GET("/", func(c *gin.Context) {
			c.HTML(http.StatusOK, "login.html", c.Request.UserAgent())
		})
		apiV1.GET("/login", autoLogin(), func(c *gin.Context) {
			c.HTML(http.StatusOK, "login.html", nil)
		})
		apiV1.POST("/login", srv.handleLogin)

		apiV1.GET("/register", func(c *gin.Context) {
			c.HTML(http.StatusOK, "register.html", nil)
		})
		apiV1.POST("/register", srv.handleRegister)

		apiV1.DELETE("/logout", func(c *gin.Context) {
			params := c.Request.URL.Query()

			if params == nil {
				c.JSON(http.StatusBadRequest, gin.H{"status": "no params specified"})
				return
			}

			for key := range params {
				// essentially overwrites and eventually gets the cookie deleted.
				c.SetCookie(key, "", 1, "/api/v1/", "", false, true) // Set the username cookie
			}
			log.Print("cookies deleted")
			c.Redirect(300, "/api/v1/login")
		})

	}

	oauth := apiV1.Group("/auth")
	{
		oauth.GET("/google", googleAuthHandler)
		oauth.GET("/google/callback", googleCallbackHandler)
		oauth.GET("/github", githubAuthHandler)
		oauth.GET("/github/callback", githubCallbackHandler)
	}

	verified := apiV1.Group("/verified")
	{
		// main pages after login
		verified.GET("/admin-panel", AuthMiddleware("admin"), func(c *gin.Context) {
			username := c.GetString("username")
			c.HTML(http.StatusOK, "admin-panel.html", gin.H{
				"username": username,
				"message":  "Welcome to the Admin Panel ",
			})
		})
		verified.GET("/dashboard", AuthMiddleware("user, admin"), func(c *gin.Context) {
			username, _ := c.Get("username")
			c.HTML(http.StatusOK, "dashboard.html", gin.H{
				"username": username,
				"message":  "Welcome to your dashboard, ",
			})
		})

		// actions for logged in users
		verified.POST("/upload", AuthMiddleware("user, admin"), srv.handleResourceUpload)
		verified.GET("/download", AuthMiddleware("user, admin"), srv.handleResourceDownload)
		verified.PATCH("/mv", AuthMiddleware("user, admin"), srv.handleResourceMove)
		verified.POST("/cp", AuthMiddleware("user, admin"), srv.handleResourceCopy)
		verified.DELETE("/rm", AuthMiddleware("user, admin"), srv.handleResourceDelete)

		admin := verified.Group("/admin")
		/* minioth will verify token no need to worry here.*/
		{
			admin.Use(AuthMiddleware("admin"))
			admin.GET("/fetch-resources", srv.handleFetchResources)
			admin.GET("/fetch-users", srv.handleFetchUsers)

			admin.POST("/useradd", srv.handleUseradd)
			admin.DELETE("/userdel", srv.handleUserdel)
			admin.PATCH("/userpatch", srv.handleUserpatch)

			admin.GET("/fetch-groups", srv.handleFetchGroups)
			admin.POST("/groupadd", srv.handleGroupadd)
			admin.DELETE("/groupdel", srv.handleGroupdel)
			admin.PATCH("/grouppatch", srv.handleGrouppatch)
			admin.POST("/hasher", srv.handleHasher)

			admin.PATCH("/chown", srv.handleResourcePerms)
			admin.PATCH("/chmod", srv.handleResourcePerms)

		}
	}

	srv.Engine.NoRoute(func(c *gin.Context) {
		c.HTML(http.StatusNotFound, "error.html", gin.H{
			"error":   "404",
			"message": "Not found",
		})
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	server := &http.Server{
		Addr:              srv.Config.Addr(srv.Config.FRONT_PORT),
		Handler:           srv.Engine,
		ReadHeaderTimeout: time.Second * 5,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	<-ctx.Done()

	stop()
	log.Println("shutting down gracefully, press Ctrl+C again to force")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown: ", err)
	}

	log.Println("Server exiting")
}
