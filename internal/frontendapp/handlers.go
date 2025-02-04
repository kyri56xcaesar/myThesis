package frontendapp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	filepath "path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

var (
	authServiceURL string
	authVersion    string

	apiServiceURL string
)

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

type PassClaim struct {
	Hashpass           string `json:"hashpass"`
	LastPasswordChange string `json:"lastPasswordChange"`
	MinPasswordAge     string `json:"minimumPasswordAge"`
	MaxPasswordAge     string `json:"maximumPasswordAge"`
	WarningPeriod      string `json:"warningPeriod"`
	InactivityPeriod   string `json:"inactivityPeriod"`
	ExpirationDate     string `json:"expirationDate"`
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

type User struct {
	Username string   `json:"username"`
	Info     string   `json:"info"`
	Home     string   `json:"home"`
	Shell    string   `json:"shell"`
	Password Password `json:"password"`
	Groups   []Group  `json:"groups"`
	Uid      int      `json:"uid"`
	Pgroup   int      `json:"pgroup"`
}

type Password struct {
	Hashpass           string `json:"hashpass"`
	LastPasswordChange string `json:"lastPasswordChange"`
	MinimumPasswordAge string `json:"minimumPasswordAge"`
	MaximumPasswordAge string `json:"maxiumPasswordAge"`
	WarningPeriod      string `json:"warningPeriod"`
	InactivityPeriod   string `json:"inactivityPeriod"`
	ExpirationDate     string `json:"expirationDate"`
}

type Group struct {
	Groupname string `json:"groupname"`
	Users     []User `json:"users"`
	Gid       int    `json:"gid"`
}

type TreeNode struct {
	Resource *Resource            `json:"resource,omitempty"`
	Children map[string]*TreeNode `json:"children,omitempty"`
	Name     string               `json:"name,omitempty"`
	Type     string               `json:"type"` // "directory" or "file"
}

type Resource struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Created_at  string `json:"created_at"`
	Updated_at  string `json:"updated_at"`
	Accessed_at string `json:"accessed_at"`
	Perms       string `json:"perms"`
	Rid         int    `json:"rid"`
	Uid         int    `json:"uid"` // as in user id (owner)
	Vid         int    `json:"vid"`
	Gid         int    `json:"gid"` // as in group id
	Pid         int    `json:"pid"` // as in parent id
	Size        int    `json:"size"`
	Links       int    `json:"links"`
}

type Volume struct {
	Path     string `json:"path"`
	Dynamic  bool   `json:"dynamic"`
	Capacity int    `json:"capacity"`
	Usage    int    `json:"usage"`
	Vid      int    `json:"vid"`
}
type UserVolume struct {
	Updated_at string `json:"updated_at"`
	Vid        int    `json:"vid"`
	Uid        int    `json:"uid"`
	Usage      int    `json:"usage"`
	Quota      int    `json:"quota"`
}

type FilePermissions struct {
	OwnerR bool
	OwnerW bool
	OwnerX bool
	GroupR bool
	GroupW bool
	GroupX bool
	OtherR bool
	OtherW bool
	OtherX bool
}

// parsePermissionsString translates something like "rwxr-xr--" into a FilePermissions struct.
func parsePermissionsString(permsStr string) FilePermissions {
	// We assume permsStr has length >= 9 (like "rwxr-xr--").
	fp := FilePermissions{}
	if len(permsStr) < 9 {
		return fp // or handle error; for safety
	}
	fp.OwnerR = permsStr[0] == 'r'
	fp.OwnerW = permsStr[1] == 'w'
	fp.OwnerX = permsStr[2] == 'x'

	fp.GroupR = permsStr[3] == 'r'
	fp.GroupW = permsStr[4] == 'w'
	fp.GroupX = permsStr[5] == 'x'

	fp.OtherR = permsStr[6] == 'r'
	fp.OtherW = permsStr[7] == 'w'
	fp.OtherX = permsStr[8] == 'x'

	return fp
}

// buildPermissionsString goes the other way around (if you need to reconstruct the string):
func buildPermissionsString(fp FilePermissions) string {
	// Convert booleans back into 'r', 'w', 'x' or '-'
	return string([]rune{
		boolChar(fp.OwnerR, 'r'),
		boolChar(fp.OwnerW, 'w'),
		boolChar(fp.OwnerX, 'x'),
		boolChar(fp.GroupR, 'r'),
		boolChar(fp.GroupW, 'w'),
		boolChar(fp.GroupX, 'x'),
		boolChar(fp.OtherR, 'r'),
		boolChar(fp.OtherW, 'w'),
		boolChar(fp.OtherX, 'x'),
	})
}

func boolChar(b bool, c rune) rune {
	if b {
		return c
	}
	return '-'
}

func (srv *HTTPService) handleLogin(c *gin.Context) {
	// only on success redirect
	var login LoginRequest

	if err := c.ShouldBind(&login); err != nil {
		log.Printf("Login binding error: %v", err)
		// Respond with the appropriate error on the template.
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad binding"})
		return
	}

	// Forward login request to the auth service
	resp, err := forwardPostRequest(authServiceURL+authVersion+"/login", "", login)
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

	c.SetCookie("access_token", authResponse.AccessToken, 3600, "/api/v1/", "", false, true)
	c.SetCookie("refresh_token", authResponse.RefreshToken, 3600, "/api/v1/", "", false, true)

	if strings.Contains(authResponse.Groups, "admin") {
		c.Redirect(http.StatusSeeOther, "/api/v1/verified/admin-panel")
	} else {
		c.Redirect(http.StatusSeeOther, "/api/v1/verified/dashboard")
	}
}

func (srv *HTTPService) handleRegister(c *gin.Context) {
	var reg RegisterRequest

	if err := c.ShouldBind(&reg); err != nil {
		log.Printf("Register binding error: %v", err)
		// Respond with the appropriate error on the template.
		c.JSON(http.StatusBadRequest, gin.H{"error": "Register binding"})
		return
	}

	// verify password repeat
	if reg.Password != reg.RepeatPassword {
		log.Printf("%v!=%v, password-repeat should match!", reg.Password, reg.RepeatPassword)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Passwords should match"})
		return
	}

	data := RegClaim{
		Username: reg.Username,
		Info:     reg.Email,
		Home:     "/home/" + reg.Username,
		Shell:    "gshell",
		Password: PassClaim{
			Hashpass: reg.Password,
		},
	}
	// Forward login request to the auth service
	resp, err := forwardPostRequest(authServiceURL+authVersion+"/register", "", gin.H{
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
		Content []User `json:"content"`
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

	// sort users on uid
	sort.Slice(resp.Content, func(i, j int) bool {
		return resp.Content[i].Uid < resp.Content[j].Uid
	})

	// answer according to format
	format := c.Request.URL.Query().Get("format")

	// Render the HTML template
	respondInFormat(c, format, resp.Content, "users_template.html")
}

func (srv *HTTPService) handleFetchGroups(c *gin.Context) {
	accessToken, err := c.Cookie("access_token")
	if err != nil {
		log.Printf("missing access_token cookie: %v", err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	req, err := http.NewRequest(http.MethodGet, authServiceURL+"/admin/groups", nil)
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
		Content []Group `json:"content"`
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

	sort.Slice(resp.Content, func(i, j int) bool {
		return resp.Content[i].Gid < resp.Content[j].Gid
	})

	// Render the HTML template

	format := c.Request.URL.Query().Get("format")

	// Render the HTML template
	respondInFormat(c, format, resp.Content, "groups_template.html")
}

func (srv *HTTPService) handleFetchVolumes(c *gin.Context) {
	accessToken, err := c.Cookie("access_token")
	if err != nil {
		log.Printf("missing access_token cookie: %v", err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	userReq, err := http.NewRequest(http.MethodGet, authServiceURL+"/admin/users", nil)
	if err != nil {
		log.Printf("failed to create request: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}
	userReq.Header.Set("Authorization", "Bearer "+accessToken)

	groupReq, err := http.NewRequest(http.MethodGet, authServiceURL+"/admin/groups", nil)
	if err != nil {
		log.Printf("failed to create request: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}
	groupReq.Header.Set("Authorization", "Bearer "+accessToken)

	volumeReq, err := http.NewRequest(http.MethodGet, apiServiceURL+"/api/v1/admin/volumes", nil)
	if err != nil {
		log.Printf("failed to create request: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}
	volumeReq.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{Timeout: 10 * time.Second}
	var (
		userResp struct {
			Content []User `json:"content"`
		}
		groupResp struct {
			Content []Group `json:"content"`
		}
		volumeResp struct {
			Content []Volume `json:"content"`
		}
	)

	var userErr, groupErr, volumeErr error

	var wg sync.WaitGroup
	wg.Add(3)

	// 1) fetch Users
	go func() {
		defer wg.Done() // signals that this goroutine is finished
		resp, err := client.Do(userReq)
		if err != nil {
			userErr = err
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			userErr = fmt.Errorf("failed to fetch users; status: %d", resp.StatusCode)
			return
		}

		if err := json.NewDecoder(resp.Body).Decode(&userResp); err != nil {
			userErr = fmt.Errorf("failed to decode users response: %v", err)
			return
		}
	}()

	// 2) fetch Groups
	go func() {
		defer wg.Done()
		resp, err := client.Do(groupReq)
		if err != nil {
			groupErr = err
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			groupErr = fmt.Errorf("failed to fetch groups; status: %d", resp.StatusCode)
			return
		}

		if err := json.NewDecoder(resp.Body).Decode(&groupResp); err != nil {
			groupErr = fmt.Errorf("failed to decode groups response: %v", err)
			return
		}
	}()

	// 3) fetch Volumes
	go func() {
		defer wg.Done()
		resp, err := client.Do(volumeReq)
		if err != nil {
			volumeErr = err
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			volumeErr = fmt.Errorf("failed to fetch volumes; status: %d", resp.StatusCode)
			return
		}
		if err := json.NewDecoder(resp.Body).Decode(&volumeResp); err != nil {
			volumeErr = fmt.Errorf("failed to decode volume response: %v", err)
			return
		}
	}()

	wg.Wait()

	if userErr != nil {
		log.Printf("user request error: %v", userErr)
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch users"})
		return
	}
	if groupErr != nil {
		log.Printf("group request error: %v", groupErr)
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch groups"})
		return
	}

	if volumeErr != nil {
		log.Printf("volume request error: %v", volumeErr)
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch volumes"})
		return
	}

	combinedData := gin.H{
		"users":   userResp.Content,
		"groups":  groupResp.Content,
		"volumes": volumeResp.Content,
	}

	format := c.Request.URL.Query().Get("format")

	// Render the HTML template
	respondInFormat(c, format, combinedData, "volumes_template.html")
}

func (srv *HTTPService) handleUseradd(c *gin.Context) {
	accessToken, err := c.Cookie("access_token")
	if err != nil {
		log.Printf("missing access_token cookie: %v", err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	var reg struct {
		Username string `form:"username" json:"username"`
		Password string `form:"password" json:"password"`
		Email    string `form:"email" json:"email"`
		Home     string `form:"home" json:"home"`
	}

	if err := c.ShouldBind(&reg); err != nil {
		log.Printf("Register binding error: %v", err)
		// Respond with the appropriate error on the template.
		c.JSON(http.StatusBadRequest, gin.H{"error": "Register binding"})
		return
	}

	data := RegClaim{
		Username: reg.Username,
		Info:     reg.Email,
		Home:     "/home/" + reg.Username,
		Shell:    "gshell",
		Password: PassClaim{
			Hashpass: reg.Password,
		},
	}
	// Forward login request to the auth service
	resp, err := forwardPostRequest(authServiceURL+authVersion+"/admin/useradd", accessToken, gin.H{
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
}

func (srv *HTTPService) handleUserdel(c *gin.Context) {
	accessToken, err := c.Cookie("access_token")
	if err != nil {
		log.Printf("missing access_token cookie: %v", err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	uid := c.Request.URL.Query().Get("uid")
	if uid == "" {
		log.Printf("missing uid parameter, must provide...")
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing uid param"})
		return
	}
	req, err := http.NewRequest(http.MethodDelete, authServiceURL+"/admin/userdel?uid="+uid, nil)
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
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to delete user"})
		return
	}
	defer response.Body.Close()

	var resp struct {
		Message string `json:"message"`
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

	// Render the HTML template
	c.String(response.StatusCode, "%v", resp.Message)
}

func (srv *HTTPService) handleUserpatch(c *gin.Context) {
	accessToken, err := c.Cookie("access_token")
	if err != nil {
		log.Printf("missing access_token cookie: %v", err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var rq struct {
		Username string `json:"username" form:"username"`
		Password string `json:"password" form:"password"`
		Home     string `json:"home" form:"home"`
		Shell    string `json:"shell" form:"shell"`
		Gid      string `json:"pgroup" form:"pgroup"`
		Groups   string `json:"groups" form:"groups"`
		Uid      int    `json:"uid" form:"uid"`
	}

	if err := c.ShouldBind(&rq); err != nil {
		log.Printf("binding error: %v", err)
		// Respond with the appropriate error on the template.
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad binding"})
		return
	}

	jsonRq, err := json.Marshal(&rq)
	if err != nil {
		log.Printf("error marshalling req body: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "error marshalling"})
		return
	}

	req, err := http.NewRequest(http.MethodPatch, authServiceURL+"/admin/userpatch", bytes.NewBuffer(jsonRq))
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
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to delete user"})
		return
	}
	defer response.Body.Close()

	if response.StatusCode >= 300 {
		log.Printf("request failed with code: %v", response.Status)
		c.JSON(response.StatusCode, gin.H{"error": "patch failed"})
		return
	}

	var resp struct {
		Message string `json:"message"`
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

	// Render the HTML template
	c.String(http.StatusOK, "%v", resp.Message)
}

func (srv *HTTPService) handleGroupadd(c *gin.Context) {
	/* Since, this is an admin function, verify early that access token exists
	* perhaps, unecessary.
	* JWT is verified at the authentication service anyhow
	* */
	accessToken, err := c.Cookie("access_token")
	if err != nil {
		log.Printf("missing access_token cookie: %v", err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req struct {
		Groupname string `json:"groupname" form:"groupname"`
	}

	if err := c.ShouldBind(&req); err != nil {
		log.Printf("failed to bind request: %v", err)
		c.JSON(400, gin.H{"error": "bad binding"})
		return
	}

	// Forward login request to the auth service
	resp, err := forwardPostRequest(authServiceURL+authVersion+"/admin/groupadd", accessToken, req)
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
}

func (srv *HTTPService) handleGroupdel(c *gin.Context) {
	accessToken, err := c.Cookie("access_token")
	if err != nil {
		log.Printf("missing access_token cookie: %v", err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	gid := c.Request.URL.Query().Get("gid")
	if gid == "" {
		log.Printf("missing gid parameter, must provide...")
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing gid param"})
		return
	}
	req, err := http.NewRequest(http.MethodDelete, authServiceURL+"/admin/groupdel?gid="+gid, nil)
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
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to delete group"})
		return
	}
	defer response.Body.Close()

	var resp struct {
		Message string `json:"message"`
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

	// Render the HTML template
	c.String(response.StatusCode, "%v", resp.Message)
}

func (srv *HTTPService) handleGrouppatch(c *gin.Context) {
}

func (srv *HTTPService) handleHasher(c *gin.Context) {
	accessToken, err := c.Cookie("access_token")
	if err != nil {
		log.Printf("missing access_token cookie: %v", err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var hashereq struct {
		HashAlg  string `json:"hashalg" form:"hashalg"`
		HashText string `json:"hash" form:"hash"`
		Text     string `json:"text" form:"text"`
		HashCost int    `json:"hashcost" form:"hashcost"`
	}

	if err := c.ShouldBind(&hashereq); err != nil {
		log.Printf("binding error: %v", err)
		// Respond with the appropriate error on the template.
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad binding"})
		return
	}

	if hashereq.Text == "" && hashereq.HashText == "" {
		c.JSON(404, gin.H{"error": "empty  request.."})
		return
	}

	jsonData, err := json.Marshal(hashereq)
	if err != nil {
		log.Printf("error marshalling request data: %v", err)
		c.JSON(500, gin.H{"error": "failed to marshal"})
		return
	}
	req, err := http.NewRequest(http.MethodPost, authServiceURL+"/admin/hasher", bytes.NewBuffer(jsonData))
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
		Result string `json:"result"`
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

	// Render the HTML template
	c.String(http.StatusOK, "%v", resp.Result)
}

/*
*********************************************************************
*   Resources
* */

func (srv *HTTPService) handleFetchResources(c *gin.Context) {
	_, err := c.Cookie("access_token")
	if err != nil {
		log.Printf("missing access_token cookie: %v", err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	struc_type := c.DefaultQuery("struct", "list")

	req, err := http.NewRequest(http.MethodGet, apiServiceURL+"/api/v1/resources?struct="+struc_type, nil)
	if err != nil {
		log.Printf("failed to create request: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}
	req.Header.Set("Access-Target", "/ 0:0")
	// req.Header.Set("Authorization", "Bearer "+acc)

	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Do(req)
	if err != nil {
		log.Printf("failed to make request: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to fetch users"})
		return
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		log.Printf("failed to read response body: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read response body"})
		return
	}

	switch struc_type {
	case "tree":
		var data map[string]interface{}
		if err := json.Unmarshal(body, &data); err != nil {
			log.Printf("failed to unmarshal response: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse response"})
			return
		}

		tree := parseTreeNode("/", data)
		if tree == nil {
			log.Printf("failed to build tree struct")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to build tree struct"})
			return
		}

		c.HTML(http.StatusOK, "tree-resources.html", tree)
	default:
		var data []Resource
		err = json.Unmarshal(body, &data)
		if err != nil {
			log.Printf("failed to unmarshal response: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse response"})
			return
		}
		c.HTML(http.StatusOK, "list-resources.html", data)
	}
}

func (srv *HTTPService) handleResourceUpload(c *gin.Context) {
	uid, _ := c.Get("user_id")
	group_ids, _ := c.Get("group_ids")

	req, err := http.NewRequest(http.MethodPost, apiServiceURL+"/api/v1/resource/upload", c.Request.Body)
	if err != nil {
		log.Printf("failed to create a new request: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	for key, values := range c.Request.Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	req.Header.Add("Access-Target", fmt.Sprintf("/ %v:%v", uid, group_ids))
	req.Header.Add("Authorization", c.Request.Header.Get("Authorization"))

	response, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("failed to forward request: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to upload resource"})
		return
	}

	defer response.Body.Close()

	c.Status(response.StatusCode)
	for key, values := range response.Header {
		for _, value := range values {
			c.Header(key, value)
		}
	}
	_, err = io.Copy(c.Writer, response.Body)
	if err != nil {
		log.Printf("failed to write response: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to write response"})
	}
}

func (srv *HTTPService) handleResourceDownload(c *gin.Context) {
	uid, _ := c.Get("user_id")
	group_ids, _ := c.Get("group_ids")
	fpath := c.Request.URL.Query().Get("target")

	if fpath == "" {
		log.Printf("must provide a target")
		c.JSON(http.StatusBadRequest, gin.H{"error": "must provide a target"})
		return
	}

	req, err := http.NewRequest(http.MethodGet, apiServiceURL+"/api/v1/resource/download", c.Request.Body)
	if err != nil {
		log.Printf("failed to create a new request: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	for key, values := range c.Request.Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	req.Header.Add("Access-Target", fmt.Sprintf("%v %v:%v", fpath, uid, group_ids))
	req.Header.Add("Authorization", c.Request.Header.Get("Authorization"))

	response, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("failed to forward request: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to upload resource"})
		return
	}

	defer response.Body.Close()
	c.Status(response.StatusCode)
	for key, values := range response.Header {
		for _, value := range values {
			c.Header(key, value)
		}
	}
	downloadName := filepath.Base(fpath)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, downloadName))
	_, err = io.Copy(c.Writer, response.Body)
	if err != nil {
		log.Printf("failed to write response: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to write response"})
	}
}

func (srv *HTTPService) handleResourceMove(c *gin.Context) {
}

func (srv *HTTPService) handleResourceDelete(c *gin.Context) {
	resource_target := c.Request.URL.Query().Get("rids")
	if resource_target == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing target parameter"})
		return
	}
	uid, _ := c.Get("user_id")
	group_ids, _ := c.Get("group_ids")

	req, err := http.NewRequest(http.MethodDelete, apiServiceURL+"/api/v1/admin/resources?rids="+resource_target, c.Request.Body)
	if err != nil {
		log.Printf("failed to create a new request: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	for key, values := range c.Request.Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	req.Header.Add("Access-Target", fmt.Sprintf("/ %v:%v", uid, group_ids))
	req.Header.Add("Authorization", c.Request.Header.Get("Authorization"))

	response, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("failed to forward request: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to upload resource"})
		return
	}

	defer response.Body.Close()

	c.Status(response.StatusCode)
	for key, values := range response.Header {
		for _, value := range values {
			c.Header(key, value)
		}
	}
	_, err = io.Copy(c.Writer, response.Body)
	if err != nil {
		log.Printf("failed to write response: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to write response"})
	}
}

func (srv *HTTPService) handleResourceCopy(c *gin.Context) {
}

func (srv *HTTPService) handleResourcePerms(c *gin.Context) {
	accessToken, err := c.Cookie("access_token")
	if err != nil {
		log.Printf("missing access_token cookie: %v", err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var (
		owner struct {
			Owner string `json:"owner"`
		}
		group struct {
			Group string `json:"group"`
		}
		permissions struct {
			FilePermissions string `json:"permissions"`
		}
		req *http.Request
	)

	err = c.BindJSON(&owner)
	if err != nil {
		err = c.Bind(&group)
		if err != nil {
			err = c.Bind(&permissions)
			if err != nil {
				log.Printf("none binding successful, bad req")
				c.JSON(http.StatusBadRequest, gin.H{"error": "bad request"})
				return
			} else {
				// perms
				req, err = http.NewRequest(http.MethodPatch, apiServiceURL+"/api/v1/admin/chmod", c.Request.Body)
				if err != nil {
					log.Printf("failed to create a request: %v", err)
					c.JSON(http.StatusInternalServerError, gin.H{"error": "fatal"})
					return
				}
			}
		} else {
			// group
			req, err = http.NewRequest(http.MethodPatch, apiServiceURL+"/api/v1/admin/chmgroup", c.Request.Body)
			if err != nil {
				log.Printf("failed to create a request: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "fatal"})
				return
			}
		}
	} else {
		// owner
		req, err = http.NewRequest(http.MethodPatch, apiServiceURL+"/api/v1/admin/chown", c.Request.Body)
		if err != nil {
			log.Printf("failed to create a request: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "fatal"})
			return
		}
	}

	whoami, exists := c.Get("user_id")
	mygroups, gexists := c.Get("groups")
	if !exists || !gexists {
		log.Printf("uid or groups were not set correctly. Authencitation fail")
		c.JSON(http.StatusInsufficientStorage, gin.H{"error": "failed auth"})
		return
	}

	req.Header.Add("Access-Target", fmt.Sprintf("/ %v:%v", whoami, mygroups))
	req.Header.Add("Authorization", "Bearer "+accessToken)

	response, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("failed to forward request: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to upload resource"})
		return
	}

	defer response.Body.Close()
}

func (srv *HTTPService) editFormHandler(c *gin.Context) {
	filename := c.Request.URL.Query().Get("filename")
	owner := c.Request.URL.Query().Get("owner")
	group := c.Request.URL.Query().Get("group")
	perms := c.Request.URL.Query().Get("perms")

	if filename == "" || owner == "" || group == "" || perms == "" {
		log.Printf("must provide args")
		c.JSON(http.StatusBadRequest, gin.H{"error": "must provide information"})
		return
	}

	// need to request for all the users and all the groups... again..
	// should implement a caching mechanism asap...
	accessToken, err := c.Cookie("access_token")
	if err != nil {
		log.Printf("missing access_token cookie: %v", err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	usersReq, err := http.NewRequest(http.MethodGet, authServiceURL+"/admin/users", nil)
	if err != nil {
		log.Printf("failed to create request: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}
	usersReq.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Do(usersReq)
	if err != nil {
		log.Printf("failed to make request: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to fetch users"})
		return
	}
	defer response.Body.Close()

	var usersResp struct {
		Content []User `json:"content"`
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		log.Printf("failed to read response body: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read response body"})
		return
	}

	err = json.Unmarshal(body, &usersResp)
	if err != nil {
		log.Printf("failed to unmarshal response: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse response"})
		return
	}

	groupsReq, err := http.NewRequest(http.MethodGet, authServiceURL+"/admin/groups", nil)
	if err != nil {
		log.Printf("failed to create request: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}
	groupsReq.Header.Set("Authorization", "Bearer "+accessToken)

	response, err = client.Do(groupsReq)
	if err != nil {
		log.Printf("failed to make request: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to fetch users"})
		return
	}
	defer response.Body.Close()

	var groupsResp struct {
		Content []Group `json:"content"`
	}

	body, err = io.ReadAll(response.Body)
	if err != nil {
		log.Printf("failed to read response body: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read response body"})
		return
	}

	err = json.Unmarshal(body, &groupsResp)
	if err != nil {
		log.Printf("failed to unmarshal response: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse response"})
		return
	}

	owner_int, err := strconv.Atoi(owner)
	if err != nil {
		log.Printf("failed to atoi owner: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad owner value"})
		return
	}
	group_int, err := strconv.Atoi(group)
	if err != nil {
		log.Printf("failed to atoi group: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad group value"})
		return
	}

	c.HTML(200, "edit-form.html", gin.H{
		"filename": filename,
		"owner":    owner_int,
		"group":    group_int,
		"perms":    parsePermissionsString(perms),
		"users":    usersResp.Content,
		"groups":   groupsResp.Content,
	})
}

/*
*********************************************************************
* */
/* helpful functions */
func forwardPostRequest(destinationURI string, accessToken string, requestData interface{}) (*http.Response, error) {
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
	req.Header.Set("Authorization", "Bearer "+accessToken)

	// Use an HTTP client to send the request
	client := &http.Client{Timeout: 10 * time.Second}
	return client.Do(req)
}

func parseTreeNode(name string, data map[string]interface{}) *TreeNode {
	if isFileNode(data) {
		var resource Resource
		jsonData, _ := json.Marshal(data)
		_ = json.Unmarshal(jsonData, &resource)

		return &TreeNode{
			Name:     name,
			Type:     "file",
			Resource: &resource,
		}
	}

	node := &TreeNode{
		Name:     name,
		Type:     "directory",
		Children: make(map[string]*TreeNode),
	}

	for key, value := range data {
		if childData, ok := value.(map[string]interface{}); ok {
			node.Children[key] = parseTreeNode(key, childData)
		}
	}

	return node
}

func isFileNode(data map[string]interface{}) bool {
	_, hasName := data["name"]
	_, hasType := data["type"]
	return hasName && hasType
}

func respondInFormat(c *gin.Context, format string, data interface{}, template_name string) {
	switch format {
	case "json":
		c.JSON(http.StatusOK, data)
	default:
		c.HTML(http.StatusOK, template_name, data)
	}
}
