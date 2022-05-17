# Go-Server
![CodeGame Version](https://img.shields.io/badge/CodeGame-v0.4-orange)
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
	"time"

	"github.com/code-game-project/go-server/cg"
)

type Game struct {
	game *cg.Game
}

func (g *Game) OnPlayerJoined(player *cg.Player) {
	fmt.Println("Player joined:", player.Username)
}

func (g *Game) OnPlayerLeft(player *cg.Player) {
	fmt.Println("Player left:", player.Username)
}

func (g *Game) OnPlayerSocketConnected(player *cg.Player, socket *cg.Socket) {
	fmt.Println("Player connected a new socket:", player.Username)
}

func (g *Game) pollEvents() {
	for {
		select {
		// Receive the next event from the events channel.
		case event := <-g.game.Events:
			g.handleEvent(event.Event, event.Player)
		default:
			return
		}
	}
}

func (g *Game) handleEvent(event cg.Event, player *cg.Player) {
	fmt.Printf("Received '%s' event from '%s'.\n", event.Name, player.Username)
}

func (g *Game) Run() {
	// Loop until the game is closed.
	for g.game.Running() {
		g.pollEvents()

		// game logic

		// 60 FPS
		time.Sleep(16 * time.Millisecond)
	}
}

func main() {
	server := cg.NewServer("my_game", cg.ServerConfig{
		Port: 8080,
		DeleteEmptyGameDuration: 15 * time.Minute,
		CGEFilepath:   "my_game.cge",
		DisplayName:   "My Game",
		Version:       "1.2.3",
		Description:   "This is my game.",
		RepositoryURL: "https://example.com",
	})

	server.Run(func(cgGame *cg.Game) {
		game := Game{
			game: cgGame,
		}

		// Register callbacks.
		cgGame.OnPlayerJoined = game.OnPlayerJoined
		cgGame.OnPlayerLeft = game.OnPlayerLeft
		cgGame.OnPlayerSocketConnected = game.OnPlayerSocketConnected

		// Run the game loop.
		game.Run()
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
