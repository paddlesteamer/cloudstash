# cloudstash
![cloudstash](https://github.com/paddlesteamer/cloudstash/workflows/cloudstash/badge.svg?branch=master)

Privacy wrapper for cloud storage(s)

## How It Works
* It automatically uploads any files you may put in the `$HOME/cloudstash` directory (automatically created on first run) to either Google Drive or Dropbox, depending on your configuration and the storage availability.
* Cloud storage providers won't be able to access the contents of your files, because they are encrypted with AES-256 before they are uploaded.
* It's an online filesystem; it doesn't store any files on your machine but you will see them as if they are.
* You can use it simultaneously from multiple machines, your changes will be synced.

## Compilation
### Ubuntu/Debian
Install `libfuse-dev` first:

```sh
$ apt install libfuse-dev
```

Then compile with `go build`

```sh
$ go build ./cmd/cloudstash
```

### MacOSX
Download and install the latest version of `FUSE for macOS` from [here](https://osxfuse.github.io/).

Then compile with `go build`
```sh
$ go build ./cmd/cloudstash
```

## Usage
Simply build & run the binary:

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

