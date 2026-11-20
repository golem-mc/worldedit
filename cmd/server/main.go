// Command server runs golem with the FastAsyncWorldEdit plugin built in.
package main

import (
	"github.com/golem-mc/golem/app"
	_ "github.com/golem-mc/worldedit"
)

func main() { app.Main() }
