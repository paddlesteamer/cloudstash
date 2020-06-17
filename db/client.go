package db

type Client struct {}

func NewClient(path string) (*Client, error) {
	return &Client{}, nil
}

func (c *Client) Close() {

}