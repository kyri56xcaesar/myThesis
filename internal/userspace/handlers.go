package userspace

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

/* A custom HTTP header parser
*
* AccessTarget=<filepath> <signature>
*
* <filepath>: the name of the file you wish to acccess
* (if it ends in /, it means its a directory)
*
* <signature>:your_user_id:[group_id,groupd_id,...]
* so the signature plainly is the user ID delimitted by ':'
* followed by the group ids (delimitted by commas).
*
*
* */
func BindAccessTarget(http_header string) (*AccessClaim, error) {
	log.Printf("trying to bind header: %s", http_header)

	parts := strings.SplitN(http_header, " ", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid header format")
	}

	target := parts[0]
	ftype := "file"
	if strings.HasSuffix(parts[0], "/") {
		ftype = "dir"
		target, _ = strings.CutSuffix(target, "/")
	}

	sig := parts[1]
	p := strings.SplitN(sig, ":", 2)
	if len(p) != 2 {
		return nil, fmt.Errorf("invalid signature format")
	}

	return &AccessClaim{
		Uid:    p[0],
		Gids:   p[1],
		Target: target,
		Type:   ftype,
	}, nil
}

/* 'resource' handlers
* SEE: models.go
* */
func (srv *UService) GetResourceHandler(c *gin.Context) {
	ac, err := BindAccessTarget(c.GetHeader("Access-Target"))
	if err != nil {
		log.Printf("failed to bind access-target: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing Access-Target header"})
		return
	}
	log.Printf("binded access claim: %+v", ac)
	/* check if claim is valid */
	/* It is checked on binding rn
	  if err := ac.validate(); err != nil {
			log.Printf("claim not valid: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	*/

	/* find target resource */
	resource, err := srv.dbh.GetResourceByFilepath(ac.Target)
	if err != nil {
		log.Printf("error retrieving resource: %v", err)
		if strings.Contains(err.Error(), "scan") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "fatal"})
		}
		return
	}

	/* Check for access authorization */
	/* This method requires Read Access to the Resource */
	if !resource.HasAccess(*ac) {
		c.JSON(http.StatusForbidden, gin.H{"error": "not allowed"})
		return
	}

	c.JSON(200, resource)
}

/*
* this should behave as:
* 'ls'
* */
func (srv *UService) ListResourcesHandler(c *gin.Context) {
	ac, err := BindAccessTarget(c.GetHeader("Access-Target"))
	if err != nil {
		log.Printf("failed to bind access-target: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing Access-Target header"})
		return
	}
	log.Printf("binded access claim: %+v", ac)

	resources, err := srv.dbh.GetAllResourcesAt(ac.Target + "/%")
	if err != nil {
		log.Printf("error retrieving resource: %v", err)
		if strings.Contains(err.Error(), "scan") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "fatal"})
		}
		return
	} else if resources == nil {
	}

	c.JSON(200, resources)
}

/* this should behave as:
* 'mkdir' for directory types,
* for file types it should trigger file upload
* simple resource
* */
func (srv *UService) PostResourcesHandler(c *gin.Context) {
	ac, err := BindAccessTarget(c.GetHeader("Access-Target"))
	if err != nil {
		log.Printf("failed to bind access-target: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing Access-Target header"})
		return
	}
	log.Printf("binded access claim: %+v", ac)

	var resources []Resource
	err = c.BindJSON(&resources)
	if err != nil {
		log.Printf("error binding: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad binding"})
		return
	}
	currentTime := time.Now().UTC().Format("2006-01-02 15:04:05-07:00")
	for i := range resources {
		resources[i].Name = ac.Target + "/" + resources[i].Name
		resources[i].Created_at = currentTime
		resources[i].Updated_at = currentTime
		resources[i].Accessed_at = currentTime
	}

	log.Printf("binded resources: %+v", resources)

	err = srv.dbh.InsertResources(resources)
	if err != nil {
		log.Printf("failed to insert resources: %v", err)
		c.JSON(422, gin.H{"error": "failed to insert resources"})
		return
	}

	c.JSON(200, gin.H{
		"message": "resources inserted",
	})
}

func (srv *UService) MoveResourcesHandler(c *gin.Context) {
	c.JSON(200, gin.H{
		"message": "tdb",
	})
}

func (srv *UService) RemoveResourcesHandler(c *gin.Context) {
	c.JSON(200, gin.H{
		"message": "tdb",
	})
}

func (srv *UService) ChmodResourceHandler(c *gin.Context) {
	c.JSON(200, gin.H{
		"message": "tbd",
	})
}

func (srv *UService) ChownResourceHandler(c *gin.Context) {
	c.JSON(200, gin.H{
		"message": "tdb",
	})
}

func (srv *UService) HandleDownload(c *gin.Context) {
	/* 1]: parse location from header*/
	ac, err := BindAccessTarget(c.GetHeader("Access-Target"))
	if err != nil {
		log.Printf("failed to bind access-target: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing Access-Target header"})
		return
	}
	log.Printf("binded access claim: %+v", ac)

	c.JSON(200, gin.H{
		"message": "tbd",
	})
}

/* This is the main endpoint handler for data uploading */
func (srv *UService) HandleUpload(c *gin.Context) {
	/* 1]: parse location from header*/
	ac, err := BindAccessTarget(c.GetHeader("Access-Target"))
	if err != nil {
		log.Printf("failed to bind access-target: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing Access-Target header"})
		return
	}
	log.Printf("binded access claim: %+v", ac)

	// 2]: we should check if destination is valid and if user is authorizated
	/*
	* */

	// 3]: determine physical destination path
	// parse the form files
	err = c.Request.ParseMultipartForm(10 << 20)
	if err != nil {
		log.Printf("failed to parse multipart form: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to parse multipart form"})
		return
	}

	/* physical path should be the target path given.
	 * This function will also perform some checks
	 */
	// lets calc the total size as well, prematurely.
	totalUploadSize := int64(0)
	for _, fileHeader := range c.Request.MultipartForm.File["files"] {
		totalUploadSize += fileHeader.Size
	}
	physicalPath, err := determinePhysicalStorage(srv.config.Volumes+ac.Target, totalUploadSize)
	if err != nil {
		log.Printf("could't establish physical storage: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "storage failure"})
		return
	}

	// 4]: perform the upload stream
	/* I would like to do this concurrently perpahps*/
	for _, fileHeader := range c.Request.MultipartForm.File["files"] {
		file, err := fileHeader.Open()
		if err != nil {
			log.Printf("failed to read uploaded file: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "fatal, failed to read uploaded files"})
			return
		}
		defer file.Close()

		outFile, err := os.Create(physicalPath)
		if err != nil {
			log.Printf("failed to create output file: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create output file"})
			return
		}
		defer outFile.Close()

		_, err = io.Copy(outFile, file)
		if err != nil {
			log.Printf("failed to save file to storage: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save file"})
			return
		}

		/* Insert the appropriate metadata as a resource */
		currentTime := time.Now().UTC().Format("2006-01-02 15:04:05-07:00")
    resource := Resource{
      Name: 
    } 

		err = srv.dbh.InsertResource(resource)
		if err != nil {
			log.Printf("failed to insert resources: %v", err)
			c.JSON(422, gin.H{"error": "failed to insert resources"})
			return
		}
	}
	c.JSON(200, gin.H{
		"message": "file/s uploaded.",
	})
}

/* this should be determined by configurating Volume destination.
*  also it will ensure the destination location exists.
* */
func determinePhysicalStorage(target string, fileSize int64) (string, error) {
	// TODO: check
	targetParts := strings.Split(target, "/")
	availableSpace, err := getAvailableSpace(strings.Join(targetParts[:2], "/"))
	if err != nil {
		return "", fmt.Errorf("failed to get available space: %v", err)
	}

	if availableSpace < uint64(fileSize) {
		return "", fmt.Errorf("insufficient space")
	}

	_, err = os.Stat(targetParts[0])
	if err != nil {
		err = os.Mkdir(targetParts[0], 0o700)
		if err != nil {
			log.Printf("failed to mkdir: %v", err)
			return "", err
		}

		_, err = os.Stat(strings.Join(targetParts[:2], "/"))
		if err != nil {
			err = os.Mkdir(strings.Join(targetParts[:2], "/"), 0o700)
			if err != nil {
				log.Printf("failed to mkdir: %v", err)
				return "", err
			}
		}
	}

	log.Printf("targetParts: %v", targetParts)
	for index, part := range targetParts[2:] {
		log.Printf("index: %v, part: %v", index, part)
		if part == "" || index == len(targetParts)-1 {
			continue
		}
		currPath := strings.Join(targetParts[:index], ",")
		_, err := os.Stat(currPath)
		if err != nil {
			err = os.Mkdir(currPath, 0o700)
			if err != nil {
				log.Printf("failed to mkdir: %v", err)
			}
		}
	}

	return target, nil
}

func getAvailableSpace(path string) (uint64, error) {
	var stat syscall.Statfs_t

	// Get filesystem stats for the given path
	err := syscall.Statfs(path, &stat)
	if err != nil {
		return 0, err
	}

	// Calculate available space in bytes
	availableSpace := stat.Bavail * uint64(stat.Bsize)
	return availableSpace, nil
}
