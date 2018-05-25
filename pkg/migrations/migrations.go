package migrations

//go:generate retool do go-bindata -pkg $GOPACKAGE -prefix "sql/" -o assets.go -ignore "\\.swp$" sql/
