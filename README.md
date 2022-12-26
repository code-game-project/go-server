# Go-Server
![CodeGame Version](https://img.shields.io/badge/CodeGame-v0.8-orange)
![Go version](https://img.shields.io/github/go-mod/go-version/code-game-project/go-server)

The Go server library for [CodeGame](https://code-game.org).

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
	cg *cg.Game
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

func (g *Game) OnSpectatorConnected(socket *cg.Socket) {
	fmt.Println("A spectator connected:", socket.Id)
}

func (g *Game) pollCommands() {
	for {
		cmd, ok := g.cg.NextCommand() // Alternatively WaitForNextCommand()
		if !ok {
			return
		}
		g.handleCommand(cmd.Origin, cmd.Cmd)
	}
}

func (g *Game) handleCommand(origin *cg.Player, cmd cg.Command) {
	fmt.Printf("Received '%s' command from '%s'.\n", cmd.Name, origin.Username)
}

func (g *Game) Run() {
	// Loop until the game is closed.
	for g.cg.Running() {
		g.pollCommands()

		// game logic

		// 60 FPS
		time.Sleep(16 * time.Millisecond)
	}
}

func main() {
	server := cg.NewServer("my_game", cg.ServerConfig{
		Port:                    8080,
		MaxPlayersPerGame:       20,
		DeleteInactiveGameDelay: 15 * time.Minute,
		KickInactivePlayerDelay: 15 * time.Minute,
		CGEFilepath:             "my_game.cge",
		WebRoot:                 "web",
		DisplayName:             "My Game",
		Version:                 "1.2.3",
		Description:             "This is my game.",
		RepositoryURL:           "https://example.com/my-game",
	})

	server.Run(func(cgGame *cg.Game, config json.RawMessage) {
		var gameConfig struct{} // should be GameConfig from cg-gen-events.
		err := json.Unmarshal(config, &gameConfig)
		cgGame.SetConfig(gameConfig)

		// Register callbacks.
		cgGame.OnPlayerJoined = game.OnPlayerJoined
		cgGame.OnPlayerLeft = game.OnPlayerLeft
		cgGame.OnPlayerSocketConnected = game.OnPlayerSocketConnected
		cgGame.OnSpectatorConnected = game.OnSpectatorConnected

		game := Game {
			cg: cgGame,
		}
		// Run the game loop.
		game.Run()
	})
}
```

## License

MIT License

Copyright (c) 2022 Julian Hofmann

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
