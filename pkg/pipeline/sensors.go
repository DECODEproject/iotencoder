package pipeline

//go:generate retool do go-bindata -pkg $GOPACKAGE -prefix "json/" -o assets.go -ignore "\\.swp$" json/

type SensorInfo struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Unit        string `json:"unit"`
}
