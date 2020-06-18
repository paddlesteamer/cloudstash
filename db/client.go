package db

type Client struct {}

func InitDB(path string) error {
	return nil
}

func NewClient(path string) (*Client, error) {
	return &Client{}, nil
}

func (c *Client) Close() {

}