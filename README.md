# Go-Server
![CodeGame Version](https://img.shields.io/badge/CodeGame-v0.1-orange)
![Go version](https://img.shields.io/github/go-mod/go-version/code-game-project/go-server)

This is the Go server library for [CodeGame](https://github.com/code-game-project).

## Installation

```sh
go get github.com/code-game-project/go-server/cg
```

## Usage

```go
package main

import (
	"fmt"

	"github.com/code-game-project/go-server/cg"
)

type Game struct {
	id     string
	server *cg.Server
}

func (g *Game) OnPlayerJoined(player *cg.Player) {
	fmt.Println("Player joined:", player.Username)
}

func (g *Game) OnPlayerLeft(player *cg.Player) {
	fmt.Println("Player left:", player.Username)
}

func (g *Game) OnPlayerEvent(player *cg.Player, event cg.Event) error {
	fmt.Println("Player event:", player.Username, event)
	return nil
}

func main() {
	server := cg.NewServer(cg.ServerConfig{
		Port: 8080,
	})

	server.Run(func(gameId string) cg.Game {
		return &Game{
			id:     gameId,
			server: server,
		}
	})
}
```

## License

MIT License

Copyright (c) 2022 CodeGame Contributors (https://github.com/orgs/code-game-project/people)

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
