package main

import (
	"log"
	"os"
	"strings"

	"github.com/Masterminds/sprig/v3"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/template/html/v2"
	"github.com/joho/godotenv"

	"{{projectName}}/internals/handlers"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.SetPrefix("{{projectName}}: ")
	log.SetOutput(os.Stdout)

	// if ENV is set to dev use godotenv
	env := os.Getenv("ENV")
	env = strings.ToLower(env)
	log.Println("ENV: ", env)
	if strings.Contains(env, "dev") {
		log.Println("Loading .env file")
		err := godotenv.Load()
		if err != nil {
			log.Fatalf("Error loading .env file: %v", err)
		}
	}
}

func main() {

	// Initialize template engine
	engine := html.New("./views", ".html")

	engine.AddFuncMap(sprig.FuncMap())

	engine.Debug(true)
	// Create app
	app := fiber.New(fiber.Config{
		Views:       engine,
		ViewsLayout: "layouts/base",
	})

	// add a middleware to log the request
	app.Use(logger.New())

	// Static files
	app.Static("/assets", "./assets/dist")

	// Theme toggle route
	app.Post("/theme/toggle", handlers.ToggleTheme)

	// Auth routes (no middleware)

	app.Get("/", handlers.GetIndex)

	// Start server
	log.Fatal(app.Listen(":3000"))
}
