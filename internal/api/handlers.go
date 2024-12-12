package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

var authServiceURL string = "http://localhost:9090"

type LoginRequest struct {
	Username string `form:"username" json:"username" binding:"required,min=3,max=20"`
	Password string `form:"password" json:"password" binding:"required,min=4,max=100"`
}

type RegisterRequest struct {
	Username       string `form:"username" json:"username" binding:"required,min=3,max=20"`
	Password       string `form:"password" json:"password" binding:"required,min=4,max=100"`
	RepeatPassword string `form:"repeat-password" json:"repeat-password" binding:"required,min=4,max=100"`
	Email          string `form:"email" json:"email"`
}

type AuthServiceResponse struct {
	Token   string `json:"token"`
	Role    string `json:"role"`
	Message string `json:"message"`
	Error   string `json:"error"`
}

func (srv *HTTPService) handleLogin(c *gin.Context) {
	// log.Printf("%v request at %v from \n%v", c.Request.Method, c.Request.URL, c.Request.UserAgent())
	// only on success redirect
	var login LoginRequest

	if err := c.ShouldBind(&login); err != nil {
		log.Printf("Login binding error: %v", err)
		// Respond with the appropriate error on the template.
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad binding"})
		return
	}

	// Forward login request to the auth service
	resp, err := forwardPostRequest(authServiceURL+"/v1/login", login)
	if err != nil {
		log.Printf("Error forwarding login request: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "forwarding fail"})
		return

	}
	defer resp.Body.Close()

	// Check the response status from the auth service
	if resp.StatusCode != http.StatusOK {
		log.Printf("Auth service returned status: %v", resp.Status)
		var ErrResp struct {
			Error string `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&ErrResp); err != nil {
			log.Printf("Error decoding auth err response: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "serious"})
			return
		}

		c.JSON(resp.StatusCode, ErrResp)
		return
	}

	// Parse the response from the auth service
	var authResponse struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		Username     string `json:"username"`
		Groups       string `json:"groups"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&authResponse); err != nil {
		log.Printf("Error decoding auth service response: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "serious"})
		return
	}

	log.Printf("response from auth: %+v", authResponse)

	c.SetCookie("username", authResponse.Username, 3600, "/api/v1/", "", false, true) // Set the username cookie
	c.SetCookie("groups", authResponse.Groups, 3600, "/api/v1/", "", false, true)
	// Save tokens in cookies
	c.SetCookie("access_token", authResponse.AccessToken, 3600, "/api/v1/", "", false, true)
	c.SetCookie("refresh_token", authResponse.RefreshToken, 3600, "/api/v1/", "", false, true)

	// Redirect user based on their role
	if strings.Contains(authResponse.Groups, "admin") {
		c.Redirect(http.StatusSeeOther, "/api/v1/verified/admin-panel")
	} else {
		c.Redirect(http.StatusSeeOther, "/api/v1/verified/dashboard")
	}
}

func (srv *HTTPService) handleRegister(c *gin.Context) {
	// log.Printf("%v request at %v from \n%v", c.Request.Method, c.Request.URL, c.Request.UserAgent())
	// log.Printf("%v request at %v from \n%v", c.Request.Method, c.Request.URL, c.Request.UserAgent())
	// only on success redirect
	var reg RegisterRequest

	if err := c.ShouldBind(&reg); err != nil {
		log.Printf("Register binding error: %v", err)
		// Respond with the appropriate error on the template.
		c.JSON(http.StatusBadRequest, gin.H{"error": "Register binding"})
		return
	}
	log.Printf("binded: %+v", reg)

	// verify password repeat
	if reg.Password != reg.RepeatPassword {
		log.Printf("%v!=%v, password-repeat should match!", reg.Password, reg.RepeatPassword)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Passwords should match"})
		return
	}
	type PassClaim struct {
		Hashpass           string `json:"hashpass"`
		LastPasswordChange string `json:"lastPasswordChange"`
		MinPasswordAge     string `json:"minimumPasswordAge"`
		MaxPasswordAge     string `json:"maximumPasswordAge"`
		WarningPeriod      string `json:"warningPeriod"`
		InactivityPeriod   string `json:"inactivityPeriod"`
		ExpirationDate     string `json:"expirationDate"`
		Length             int    `json:"passwordLength"`
	}

	type RegClaim struct {
		Username string    `json:"username"`
		Info     string    `json:"info"`
		Home     string    `json:"home"`
		Shell    string    `json:"shell"`
		Password PassClaim `json:"password"`
		Uid      int       `json:"uid"`
		Pgroup   int       `json:"pgroup"`
	}

	data := RegClaim{
		Username: reg.Username,
		Info:     reg.Email,
		Home:     "/home/" + reg.Username,
		Shell:    "gshell",
		Password: PassClaim{
			Hashpass: reg.Password,
			Length:   len(reg.Password),
		},
	}
	// Forward login request to the auth service
	resp, err := forwardPostRequest(authServiceURL+"/v1/register", gin.H{
		"user": data,
	})
	if err != nil {
		log.Printf("Error forwarding register request: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "forwarding fail"})
		return

	}
	defer resp.Body.Close()

	// Check the response status from the auth service
	if resp.StatusCode != http.StatusOK {
		log.Printf("Auth service returned status: %v", resp.Status)
		var ErrResp struct {
			Error string `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&ErrResp); err != nil {
			log.Printf("Error decoding auth err response: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "serious"})
			return
		}

		c.JSON(resp.StatusCode, ErrResp)
		return
	}

	// Parse the response from the auth service
	var authResponse struct {
		Login_url string `json:"login_url"`
		Message   string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&authResponse); err != nil {
		log.Printf("Error decoding auth service response: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "serious"})
		return
	}

	log.Printf("response from auth: %+v", authResponse)

	// at this point registration should be successful, we can directly login the user
	// .. somehow

	time.Sleep(2 * time.Second)
	c.Redirect(303, "/api/v1/login")
}

func (srv *HTTPService) handleFetchUsers(c *gin.Context) {
	accessToken, err := c.Cookie("access_token")
	if err != nil {
		log.Printf("missing access_token cookie: %v", err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	req, err := http.NewRequest(http.MethodGet, authServiceURL+"/admin/users", nil)
	if err != nil {
		log.Printf("failed to create request: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Do(req)
	if err != nil {
		log.Printf("failed to make request: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to fetch users"})
		return
	}
	defer response.Body.Close()

	var resp struct {
		Content []string `json:"content"`
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		log.Printf("failed to read response body: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read response body"})
		return
	}

	err = json.Unmarshal(body, &resp)
	if err != nil {
		log.Printf("failed to unmarshal response: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse response"})
		return
	}

	log.Printf("Fetched users: %+v", resp)

	// Render the HTML template
	c.HTML(http.StatusOK, "users_template.html", parseUserData(resp.Content))
}

func forwardPostRequest(destinationURI string, requestData interface{}) (*http.Response, error) {
	// Marshal the request data into JSON
	jsonData, err := json.Marshal(requestData)
	if err != nil {
		return nil, fmt.Errorf("error marshalling request data: %w", err)
	}

	// Create a new POST request with the JSON data
	req, err := http.NewRequest(http.MethodPost, destinationURI, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Use an HTTP client to send the request
	client := &http.Client{Timeout: 10 * time.Second}
	return client.Do(req)
}

type User struct {
	Username     string
	Info         string
	Home         string
	Shell        string
	UserID       string
	PrimaryGroup string
	Groups       string
}

func parseUserData(data []string) []User {
	var users []User
	for _, entry := range data {
		entry = strings.Trim(entry, "{}")    // Remove curly braces
		fields := strings.Split(entry, ", ") // Split by ", "
		if len(fields) == 6 {                // Ensure there are 6 fields
			users = append(users, User{
				Username:     fields[0],
				Info:         fields[1],
				Home:         fields[2],
				Shell:        fields[3],
				UserID:       fields[4],
				PrimaryGroup: fields[5],
				Groups:       fields[6],
			})
		}
	}
	return users
}
