package main

import (
	"github.com/joho/godotenv"
	"mahresources/application_context"
)

func main() {
	_ = godotenv.Load(".env")

	context, db, _ := application_context.CreateContext()

	println(context, db)

}
