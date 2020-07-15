package common

import "github.com/paddlesteamer/go-fuse-c/fuse"

const (
	DrvFile   = fuse.S_IFREG
	DrvFolder = fuse.S_IFDIR

	DatabaseFileName = "cloudstash.sqlite3"

	cacheFilePrefix = "cloudstash-cached-"
	dbFilePrefix    = "cloudstash-db-"
)

const (
	DropboxAppKey     = "l4v6ipcr1rlwu1x"
	GDriveCredentials = `{
		"installed":
			{
				"client_id":"731677456506-pm15gpb5d2c71ielkf2bkcu2d638tj12.apps.googleusercontent.com",
				"project_id":"cloudstash",
				"auth_uri":"https://accounts.google.com/o/oauth2/auth",
				"token_uri":"https://oauth2.googleapis.com/token",
				"auth_provider_x509_cert_url":"https://www.googleapis.com/oauth2/v1/certs",
				"client_secret":"RfpLjMKVJX6rI_OW5DVmJTlT",
				"redirect_uris":[
					"http://localhost:48500/gdrive/redirect"
				]
			}
		}`
	GDriveAppFolder = "cloudstash"
)
