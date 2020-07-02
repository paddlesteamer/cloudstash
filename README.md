# cloudstash
![cloudstash](https://github.com/paddlesteamer/cloudstash/workflows/cloudstash/badge.svg?branch=master)

Privacy wrapper for cloud storage(s)

## Compilation
Install libfuse-dev first:

```sh
$ apt install libfuse-dev
```

Then compile with `go build`

```sh
$ go build ./cmd/cloudstash
```

### Important but Temporary Notice
You need to clone `github.com/paddlesteamer/go-fuse-c` as `github.com/vgough/go-fuse-c` because `vgough/go-fuse-c` doesn't implement an unmount method. There is a [PR]("https://github.com/vgough/go-fuse-c/pull/2") open right now. This issue will be resolved depending on if PR will be merged or not.  

```bash
cd "$(go env GOPATH)/pkg/mod/github.com/vgough"
rm -rf go-fuse-c@v0.7.1
git clone github.com/paddlesteamer/go-fuse-c go-fuse-c@v0.7.1
```

## Usage
Simply run the binary:

```sh
$ go run ./cmd/cloudstash
```

If you want to use config directory other than the default directory `~/.config/cloudstash`, you can specify it with `-c`:

```sh
$ go run ./cmd/cloudstash -c <my directory>
```

Or if you want to use mount point other than the default mount point `~/cloudstash`, you can also specify it:

```sh
$ go run ./cmd/cloudstash -m <another directory>
```

## Disclaimer
Still in development, don't rely on it since there are many known bugs.
