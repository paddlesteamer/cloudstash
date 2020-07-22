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
Can cause file loss on heavy concurrent use (i.e. copying lots of files into same folder at the same time from different machines) but, otherwise, it will most probabaly hold.  
