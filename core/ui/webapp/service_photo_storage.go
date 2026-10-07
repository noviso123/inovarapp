package webapp

import (
	"errors"
	"strings"
)

type localServicePhoto struct {
	ID        string `json:"id"`
	ServiceID string `json:"serviceId"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Size      int64  `json:"size"`
	DataURL   string `json:"dataUrl"`
	CreatedAt string `json:"createdAt"`
}

const localServicePhotoPrefix = "local:"

var errLocalPhotoStorageUnavailable = errors.New("local photo storage is unavailable")

func mergeServicePhotos(remote []appliancePhoto, local []localServicePhoto) []appliancePhoto {
	photos := append([]appliancePhoto(nil), remote...)
	for _, photo := range local {
		photos = append(photos, appliancePhoto{
			Name: photo.Name, Path: localServicePhotoPrefix + photo.ID, URL: photo.DataURL, Local: true,
		})
	}
	return photos
}

func localServicePhotoID(path string) (string, bool) {
	if !strings.HasPrefix(path, localServicePhotoPrefix) {
		return "", false
	}
	id := strings.TrimPrefix(path, localServicePhotoPrefix)
	return id, id != ""
}
