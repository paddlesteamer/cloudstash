package common

import "github.com/paddlesteamer/go-fuse-c/fuse"

const (
	DRV_FILE   = fuse.S_IFREG
	DRV_FOLDER = fuse.S_IFDIR

	cacheFilePrefix = "cloudstash-cached-"
	dbFilePrefix    = "cloudstash-db-"
)

const (
	DROPBOX_APP_KEY = "l4v6ipcr1rlwu1x"
	DATABASE_FILE   = "dropbox://cloudstash.sqlite3" // @TODO: to be removed later
)

const (
	GDRIVE_CREDENTIALS = `{
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
)
