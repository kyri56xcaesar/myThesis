package userspace

import (
	"encoding/json"
	"fmt"
	"io"
	ut "kyri56xcaesar/myThesis/internal/utils"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

/* HTTP Gin handlers related to volumes, uservolumes, groupvolumes, used by the api to handle endpoints*/

func (srv *UService) HandleVolumes(c *gin.Context) {
	switch c.Request.Method {
	case http.MethodGet:

		vid := c.Request.URL.Query().Get("vid")
		if vid != "" {
			vid_int, err := strconv.Atoi(vid)
			if err != nil {
				log.Printf("failed to atoi vid: %v", err)
				c.JSON(http.StatusBadRequest, gin.H{"error": "bad vid"})
				return
			}
			volume, err := srv.dbh.vdh.GetVolumeByVid(vid_int)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"content": volume})
		} else {
			volumes, err := srv.dbh.vdh.GetVolumes()
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "bad request"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"content": volumes})
		}

	case http.MethodPut:
		c.JSON(200, gin.H{"status": "tbd"})
	case http.MethodDelete:
		vid := c.Request.URL.Query().Get("vid")
		if vid == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "must provide vid"})
			return
		}
		vid_int, err := strconv.Atoi(vid)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "couldn't convert to int"})
			return
		}

		err = srv.dbh.vdh.DeleteVolume(vid_int)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete the volume"})
			return
		}

		c.JSON(http.StatusAccepted, gin.H{"status": "successfully deleted volume"})

	case http.MethodPatch:
		c.JSON(200, gin.H{"status": "tbd"})
	case http.MethodPost:
		var volumes []Volume
		err := c.BindJSON(&volumes)
		if err != nil {
			log.Printf("failed to bind volumes: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "couldn't bind volumes"})
			return
		}
		err = srv.dbh.vdh.InsertVolumes(volumes)
		if err != nil {
			log.Printf("failed to insert volumes: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "couldn't insert volumes"})
			return
		}
		c.JSON(http.StatusCreated, gin.H{"error": "inserted volume(s)"})
	default:
		c.JSON(http.StatusMethodNotAllowed, gin.H{"error": "not allowed."})
	}
}

func (srv *UService) HandleUserVolumes(c *gin.Context) {
	switch c.Request.Method {
	case http.MethodPost:
		var (
			userVolumes []UserVolume
			userVolume  UserVolume
		)

		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			log.Printf("failed to read request body: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
			return
		}

		err = json.Unmarshal(body, &userVolumes)
		if err != nil {
			err = json.Unmarshal(body, &userVolume)
			// single userVolume
			if err != nil {
				log.Printf("fail to bind body: %v", err)
				c.JSON(http.StatusBadRequest, gin.H{"error": "bad request, failed to bind"})
				return
			}

			if capacity, _ := strconv.ParseFloat(srv.config.VCapacity, 64); int(userVolume.Quota) == 0 || userVolume.Quota > float64(capacity) {
				log.Printf("inserted")
				userVolume.Quota = float64(capacity)
			}

			err = srv.dbh.uvdh.InsertUserVolume(userVolume)
			if err != nil {
				log.Printf("failed to insert user volume: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to insert uv"})
				return
			}
			c.JSON(http.StatusCreated, gin.H{"status": "inserted user volume"})
			return
		}
		// binded user
		err = srv.dbh.uvdh.InsertUserVolumes(userVolumes)
		if err != nil {
			log.Printf("failed to insert user volumes: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to insert uv"})
			return
		}
		c.JSON(http.StatusCreated, gin.H{"status": "inserted user volumes"})

	case http.MethodDelete:
	case http.MethodPatch:

	case http.MethodGet:
		uids := c.Request.URL.Query().Get("uids")
		vids := c.Request.URL.Query().Get("vids")
		var (
			userVolumes []UserVolume
			data        interface{}
			err         error
		)

		if uids == "" && vids == "" {
			// return all
			data, err = srv.dbh.uvdh.GetUserVolumes()
			if err != nil {
				log.Printf("failed to retrieve user volumes: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve user volumes"})
				return
			}
			userVolumes = data.([]UserVolume)
		} else if uids == "" {
			// return by vids
			data, err = srv.dbh.uvdh.GetUserVolumesByVolumeIds(strings.Split(strings.TrimSpace(vids), ","))
			if err != nil {
				log.Printf("failed to retrieve user volumes: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve user volumes"})
				return
			}
			userVolumes = data.([]UserVolume)
		} else if vids == "" {
			// return by uids
			data, err = srv.dbh.uvdh.GetUserVolumesByUserIds(strings.Split(strings.TrimSpace(uids), ","))
			if err != nil {
				log.Printf("failed to retrieve user volumes: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve user volumes"})
				return
			}
			userVolumes = data.([]UserVolume)
		} else {
			// return by both
			data, err = srv.dbh.uvdh.GetUserVolumesByUidsAndVids(strings.Split(strings.TrimSpace(uids), ","), strings.Split(strings.TrimSpace(vids), ","))
			if err != nil {
				log.Printf("failed to retrieve user volumes: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve user volumes"})
				return
			}
			userVolumes = data.([]UserVolume)
		}
		c.JSON(http.StatusOK, gin.H{"content": userVolumes})

	default:
		c.JSON(http.StatusMethodNotAllowed, gin.H{"error": "method not allowed"})
	}
}

func (srv *UService) HandleGroupVolumes(c *gin.Context) {
	switch c.Request.Method {
	case http.MethodPost:
		var (
			groupVolumes []GroupVolume
			groupVolume  GroupVolume
		)

		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			log.Printf("failed to read request body: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
			return
		}

		err = json.Unmarshal(body, &groupVolumes)
		if err != nil {
			log.Printf("didn't bind groupVolumes, lets try a groupVolume..")
			err = json.Unmarshal(body, &groupVolume)
			// single userVolume
			if err != nil {
				log.Printf("fail to bind body: %v", err)
				c.JSON(http.StatusBadRequest, gin.H{"error": "bad request, failed to bind"})
				return
			}

			if capacity, _ := strconv.ParseFloat(srv.config.VCapacity, 64); int(groupVolume.Quota) == 0 || groupVolume.Quota > float64(capacity) {
				log.Printf("inserted")
				groupVolume.Quota = float64(capacity)
			}

			err = srv.dbh.gvdh.InsertGroupVolume(groupVolume)
			if err != nil {
				log.Printf("failed to insert group volume: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to insert gv"})
				return
			}
			c.JSON(http.StatusCreated, gin.H{"status": "inserted group volume"})
			return
		}

		// binded user
		err = srv.dbh.gvdh.InsertGroupVolumes(groupVolumes)
		if err != nil {
			log.Printf("failed to insert group volumes: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to insert gv"})
			return
		}
		c.JSON(http.StatusCreated, gin.H{"status": "inserted group volumes"})

	case http.MethodDelete:
	case http.MethodPatch:
	case http.MethodGet:
		gids := c.Request.URL.Query().Get("gids")
		vids := c.Request.URL.Query().Get("vids")
		var (
			groupVolumes []GroupVolume
			data         interface{}
			err          error
		)

		if gids == "" && vids == "" {
			// return all
			data, err = srv.dbh.gvdh.GetGroupVolumes()
			if err != nil {
				log.Printf("failed to retrieve group volumes: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve group volumes"})
				return
			}
			groupVolumes = data.([]GroupVolume)
		} else if gids == "" {
			// return by vids
			data, err = srv.dbh.gvdh.GetGroupVolumesByVolumeIds(strings.Split(strings.TrimSpace(vids), ","))
			if err != nil {
				log.Printf("failed to retrieve group volumes: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve group volumes"})
				return
			}
			groupVolumes = data.([]GroupVolume)
		} else if vids == "" {
			// return by uids
			data, err = srv.dbh.gvdh.GetGroupVolumesByGroupIds(strings.Split(strings.TrimSpace(gids), ","))
			if err != nil {
				log.Printf("failed to retrieve group volumes: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve group volumes"})
				return
			}
			groupVolumes = data.([]GroupVolume)
		} else {
			// return by both
			data, err = srv.dbh.gvdh.GetGroupVolumesByVidsAndGids(strings.Split(strings.TrimSpace(vids), ","), strings.Split(strings.TrimSpace(gids), ","))
			if err != nil {
				log.Printf("failed to retrieve group volumes: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retireve group volumes"})
				return
			}
			groupVolumes = data.([]GroupVolume)
		}
		c.JSON(http.StatusOK, gin.H{"content": groupVolumes})

	default:
		c.JSON(http.StatusMethodNotAllowed, gin.H{"error": "method not allowed"})
	}
}

func (srv *UService) ClaimVolumeSpace(size int64, ac AccessClaim) error {
	// for now:
	ac.Vid = 1
	uid, err := strconv.Atoi(ac.Uid)
	if err != nil {
		log.Printf("failed to atoi ac.Uid, shouldn't have passed till here tbh...:%v", err)
		return fmt.Errorf("atoi failure, shouldn't be here: %v", err)
	}
	volume, err := srv.dbh.vdh.GetVolumeByVid(ac.Vid)
	if err != nil {
		log.Printf("could not retrieve volume: %v", err)
		return fmt.Errorf("could not retrieve volume: %w", err)
	}
	// check for current volume usage.
	// size is in Bytes
	size_inGB := float64(size) / 1000000000
	new_usage_inGB := volume.Usage + size_inGB

	if new_usage_inGB > volume.Capacity {
		log.Printf("volume is full.")
		return fmt.Errorf("claim exceeds capacity")
	}

	// if not dynamic, we should check for per user/group quota
	uv, err := srv.dbh.uvdh.GetUserVolumeByUid(uid)
	if err != nil {
		log.Printf("failed to retrieve user volume: %v", err)
		return err
	}
	gids := strings.Split(strings.TrimSpace(ac.Gids), ",")
	log.Printf("gids: %v, ac: %+v", gids, ac)
	if len(gids) == 0 {
		return fmt.Errorf("empty gids, shouldn't be here: %v", err)
	}
	gvs, err := srv.dbh.gvdh.GetGroupVolumesByGroupIds(gids)
	if err != nil {
		log.Printf("failed to retrieve group volume: %v", err)
		return err
	}

	// update all usages
	// volume
	// volume claims user/group
	uv.Usage += size_inGB
	gvs_casted := gvs.([]GroupVolume)
	for index, gv := range gvs_casted {
		log.Printf("gv: %+v", gv)
		gvs_casted[index].Usage += size_inGB
	}
	volume.Usage = new_usage_inGB

	log.Printf("updated volume: %+v", volume)
	log.Printf("updated uv: %+v", uv)
	log.Printf("updated gvs: %+v", gvs_casted)

	err = srv.dbh.vdh.UpdateVolume(volume)
	if err != nil {
		log.Printf("failed to update volume usages: %v", err)
		return err
	}
	err = srv.dbh.uvdh.UpdateUserVolume(uv)
	if err != nil {
		log.Printf("failed to update user volume usages: %v", err)
		return err
	}
	err = srv.dbh.gvdh.UpdateGroupVolumes(gvs_casted)
	if err != nil {
		log.Printf("failed to update group volume usages: %v", err)
		return err
	}

	return nil
}

func (srv *UService) ReleaseVolumeSpace(size int64, ac AccessClaim) error {
	// for now:
	ac.Vid = 1
	uid, err := strconv.Atoi(ac.Uid)
	if err != nil {
		log.Printf("failed to atoi ac.Uid, shouldn't have passed till here tbh...:%v", err)
		return fmt.Errorf("atoi failure, shouldn't be here: %v", err)
	}
	volume, err := srv.dbh.vdh.GetVolumeByVid(ac.Vid)
	if err != nil {
		log.Printf("could not retrieve volume: %v", err)
		return fmt.Errorf("could not retrieve volume: %w", err)
	}

	size_inGB := float64(size) / 1000000000
	new_usage_inGB := volume.Usage - size_inGB

	if new_usage_inGB < 0 {
		new_usage_inGB = 0
	}

	uv, err := srv.dbh.uvdh.GetUserVolumeByUid(uid)
	if err != nil {
		log.Printf("failed to retrieve user volume: %v", err)
		return err
	}
	gid, err := strconv.Atoi(strings.Split(strings.TrimSpace(ac.Gids), ",")[0])
	if err != nil {
		return fmt.Errorf("atoi failure, shouldn't be here: %v", err)
	}
	gv, err := srv.dbh.gvdh.GetGroupVolumeByGid(gid)
	if err != nil {
		log.Printf("failed to retrieve group volume: %v", err)
		return err
	}

	// update all usages
	// volume
	// volume claims user/group
	uv.Usage -= size_inGB
	gv.Usage -= size_inGB
	volume.Usage = new_usage_inGB

	err = srv.dbh.vdh.UpdateVolume(volume)
	if err != nil {
		log.Printf("failed to update volume usages: %v", err)
		return err
	}
	err = srv.dbh.uvdh.UpdateUserVolume(uv)
	if err != nil {
		log.Printf("failed to update user volume usages: %v", err)
		return err
	}
	err = srv.dbh.gvdh.UpdateGroupVolume(gv)
	if err != nil {
		log.Printf("failed to update group volume usages: %v", err)
		return err
	}

	return nil
}

/* this should be determined by configurating Volume destination.
*  also it will ensure the destination location exists.
* */
func determinePhysicalStorage(target string, fileSize int64) (string, error) {
	targetParts := strings.Split(target, "/")
	availableSpace, err := ut.GetAvailableSpace(strings.Join(targetParts[:2], "/"))
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

	for index, part := range targetParts[2:] {
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
