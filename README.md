# hdn_drv
![hdn-drv](https://github.com/paddlesteamer/hdn-drv/workflows/hdn-drv/badge.svg?branch=master)

Privacy wrapper for cloud storage(s)

## Compilation
Install libfuse-dev first:

```sh
$ apt install libfuse-dev
```

Then compile with `go build`

```sh
$ go build ./cmd/hdn-drv
```

## Usage
Simply run the binary:

```sh
$ go run ./cmd/hdn-drv
```

If you want to use config directory other than the default directory `~/.config/hdn-drv`, you can specify it with `-c`:

```sh
$ go run ./cmd/hdn-drv -c <my directory>
```

Or if you want to use mount point other than the default mount point `~/hdn-drv`, you can also specify it:

```sh
$ go run ./cmd/hdn-drv -m <another directory>
```

## Disclaimer
Still in development, don't rely on it since there are many known bugs.
