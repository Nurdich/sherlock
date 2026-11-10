// Copyright (c) 2019 Sherlock Project
// SPDX-License-Identifier: MIT
//
// Go port of the Sherlock Project: https://github.com/sherlock-project/sherlock

package main

import (
	"os"

	"github.com/Nurdich/sherlock/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:]))
}
