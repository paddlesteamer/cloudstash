# cloudstash
![cloudstash](https://github.com/paddlesteamer/cloudstash/workflows/cloudstash/badge.svg?branch=master)

Privacy wrapper for cloud storage(s)

## What Does It Do
It will create a folder under your home named **cloudstash** and share the files you put in this folder between Google Drive and Dropbox. It is an online filesystem, so it doesn't keep your files on your machine but you will see them as they are. The uploaded files are encrypted with 256 bit AES-CTR and that's why the cloud storages won't be able to access the contents of your files.

You can use **cloudstash** in different machines, your changes will be synced. 

## Compilation
### For Ubuntu/Debian
Install `libfuse-dev` first:

```sh
$ apt install libfuse-dev
```

Then compile with `go build`

```sh
$ go build ./cmd/cloudstash
```

### For MacOSX
Download and install `osxfuse` from  [here](https://osxfuse.github.io/).  It is enough to install the latest version of `FUSE for macOS`.  Then compile with `go build`

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

