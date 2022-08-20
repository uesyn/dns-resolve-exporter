package main

import (
	"os"
)

func main() {
	if err := MainApp().Run(os.Args); err != nil {
		Logger().Errorw("failed to execute", "error", err)
	}
}
