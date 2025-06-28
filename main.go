package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

//go:embed templates/*
var templateFiles embed.FS

var (
	// These will be injected by GoReleaser
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
	builtBy = "unknown"

	cfgFile string
	rootCmd = &cobra.Command{
		Use:   "initgo",
		Short: "Initialize a new Go project with go mod init and main.go",
		Long: `A CLI tool to automate Go project initialization.
		
This tool will:
- Execute 'go mod init' with the specified project name
- Create a main.go file with proper logging setup
- Handle errors gracefully with informative messages`,
		RunE: runInit,
	}

	webappCmd = &cobra.Command{
		Use:   "webapp [project-name]",
		Short: "Create a new web application with Go Fiber, HTMX, Alpine.js, and Tailwind CSS",
		Long: `Create a new web application using the modern stack:
- Go with Fiber v2 framework
- Fiber HTML templates  
- HTMX for seamless interactions
- Alpine.js for reactive components
- Tailwind CSS with DaisyUI for styling
- Hot reload with Air
- Asset bundling with esbuild

If no project name is provided, the current directory name will be used
and the application will be initialized in the current directory.`,
		RunE: runWebapp,
	}

	versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Print the version information",
		Long:  "Print the version, commit hash, build date, and builder information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("initgo %s\n", version)
			fmt.Printf("  Commit: %s\n", commit)
			fmt.Printf("  Date: %s\n", date)
			fmt.Printf("  Built by: %s\n", builtBy)
		},
	}
)

type TemplateData struct {
	ProjectName   string
	ModuleName    string
	AppTitle      string
	AppTitleCamel string
}

func init() {
	cobra.OnInitialize(initConfig)

	// Set version for --version flag
	rootCmd.Version = version

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.initgo.yaml)")

	// Add commands
	rootCmd.AddCommand(webappCmd)
	rootCmd.AddCommand(versionCmd)
}

func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		// Search config in home directory with name ".initgo" (without extension)
		viper.AddConfigPath(home)
		viper.SetConfigType("yaml")
		viper.SetConfigName(".initgo")
	}

	// If a config file is found, read it in
	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}

func runInit(cmd *cobra.Command, args []string) error {
	var projectName string

	// Determine project name
	if len(args) > 0 {
		projectName = args[0]
	} else {
		// Use current directory name as default
		currentDir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
		projectName = filepath.Base(currentDir)
	}

	// Validate project name
	if projectName == "" {
		return fmt.Errorf("project name cannot be empty")
	}

	// Clean project name (remove any invalid characters for go mod)
	projectName = strings.TrimSpace(projectName)

	fmt.Printf("Initializing Go module with name '%s'...\n", projectName)

	// Execute go mod init
	if err := executeGoModInit(projectName); err != nil {
		return fmt.Errorf("failed to initialize go module: %w", err)
	}

	fmt.Println("Creating main.go...")

	// Create main.go file
	if err := createMainGo(projectName); err != nil {
		return fmt.Errorf("failed to create main.go: %w", err)
	}

	fmt.Println("Done!")
	return nil
}

func runWebapp(cmd *cobra.Command, args []string) error {
	var projectName string
	var createNewDir bool

	// Determine project name
	if len(args) > 0 {
		projectName = args[0]
		createNewDir = true
	} else {
		// Use current directory name as default
		currentDir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
		projectName = filepath.Base(currentDir)
		createNewDir = false
	}

	// Validate project name
	if projectName == "" {
		return fmt.Errorf("project name cannot be empty")
	}

	// Clean project name
	projectName = strings.TrimSpace(projectName)

	if createNewDir {
		fmt.Printf("Creating web application '%s'...\n", projectName)

		// Create project directory if it doesn't exist
		if _, err := os.Stat(projectName); os.IsNotExist(err) {
			if err := os.MkdirAll(projectName, 0755); err != nil {
				return fmt.Errorf("failed to create project directory: %w", err)
			}
		}
	} else {
		fmt.Printf("Initializing web application '%s' in current directory...\n", projectName)
	}

	// Store current directory to return to later
	originalDir, _ := os.Getwd()

	// Change to project directory if we created a new one
	if createNewDir {
		if err := os.Chdir(projectName); err != nil {
			return fmt.Errorf("failed to change to project directory: %w", err)
		}
	}

	// Prepare template data
	templateData := TemplateData{
		ProjectName:   projectName,
		ModuleName:    projectName,
		AppTitle:      cases.Title(language.English).String(strings.ReplaceAll(projectName, "-", " ")),
		AppTitleCamel: toCamelCase(projectName),
	}

	// Generate files from templates
	if err := generateWebappFiles(templateData); err != nil {
		if createNewDir {
			os.Chdir(originalDir)
		}
		return fmt.Errorf("failed to generate files: %w", err)
	}

	// Rename .env.example to .env if it exists
	if err := renameEnvExample(); err != nil {
		// Don't fail the entire process if .env.example doesn't exist
		fmt.Printf("Note: %v\n", err)
	}

	// Initialize go module if go.mod doesn't exist
	if _, err := os.Stat("go.mod"); os.IsNotExist(err) {
		fmt.Println("Initializing Go module...")
		if err := executeGoModInit(projectName); err != nil {
			if createNewDir {
				os.Chdir(originalDir)
			}
			return fmt.Errorf("failed to initialize go module: %w", err)
		}
	} else {
		fmt.Println("Go module already exists, skipping go mod init...")
	}

	// Install Go dependencies
	fmt.Println("Installing Go dependencies...")
	if err := installGoDependencies(); err != nil {
		if createNewDir {
			os.Chdir(originalDir)
		}
		return fmt.Errorf("failed to install Go dependencies: %w", err)
	}

	// Install Node.js dependencies
	fmt.Println("Installing Node.js dependencies...")
	if err := installNodeDependencies(); err != nil {
		if createNewDir {
			os.Chdir(originalDir)
		}
		return fmt.Errorf("failed to install Node.js dependencies: %w", err)
	}

	// Return to original directory if we created a new one
	if createNewDir {
		os.Chdir(originalDir)
	}

	fmt.Println("✅ Web application created successfully!")
	if createNewDir {
		fmt.Printf(`
Next steps:
1. cd %s
2. pnpm run dev (in one terminal - for asset building)
3. air (in another terminal - for Go hot reload)
4. Open http://localhost:3000

Your web application is ready with:
- Go Fiber v2 backend
- HTMX + Alpine.js frontend
- Tailwind CSS + DaisyUI styling
- Hot reload setup
`, projectName)
	} else {
		fmt.Printf(`
Next steps:
1. pnpm run dev (in one terminal - for asset building)
2. air (in another terminal - for Go hot reload)
3. Open http://localhost:3000

Your web application '%s' is ready with:
- Go Fiber v2 backend
- HTMX + Alpine.js frontend
- Tailwind CSS + DaisyUI styling
- Hot reload setup
`, projectName)
	}

	return nil
}

func generateWebappFiles(data TemplateData) error {
	fmt.Println("Generating files from templates...")

	// Walk through all template files
	return fs.WalkDir(templateFiles, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if d.IsDir() {
			return nil
		}

		// Get relative path from templates/
		relPath, err := filepath.Rel("templates", path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}

		// Determine output path
		outputPath := relPath

		// Handle template files (.tmpl extension)
		if strings.HasSuffix(relPath, ".tmpl") {
			outputPath = strings.TrimSuffix(relPath, ".tmpl")
		}

		// Create output directory if needed
		outputDir := filepath.Dir(outputPath)
		if outputDir != "." {
			if err := os.MkdirAll(outputDir, 0755); err != nil {
				return fmt.Errorf("failed to create output directory %s: %w", outputDir, err)
			}
		}

		// Read template content
		content, err := templateFiles.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read template %s: %w", path, err)
		}

		// Process all files to replace {{projectName}} placeholders
		processedContent, err := processAllFiles(string(content), data)
		if err != nil {
			return fmt.Errorf("failed to process file %s: %w", path, err)
		}
		content = []byte(processedContent)

		// Write output file
		if err := os.WriteFile(outputPath, content, 0644); err != nil {
			return fmt.Errorf("failed to write file %s: %w", outputPath, err)
		}

		fmt.Printf("  ✓ Created %s\n", outputPath)
		return nil
	})
}

func shouldProcessAsTemplate(relPath string) bool {
	// Process all files now - this function is kept for backward compatibility
	// but is no longer used in the main logic
	return true
}

func processAllFiles(content string, data TemplateData) (string, error) {
	// Simple string replacement for {{projectName}} placeholders
	// This avoids conflicts with HTML files that contain Fiber template syntax

	// Replace {{projectName}} with the actual project name
	content = strings.ReplaceAll(content, "{{projectName}}", data.ProjectName)

	// Replace {{moduleName}} with the module name
	content = strings.ReplaceAll(content, "{{moduleName}}", data.ModuleName)

	// Replace {{appTitle}} with the app title
	content = strings.ReplaceAll(content, "{{appTitle}}", data.AppTitle)

	// Replace {{appTitleCamel}} with the camel case app title
	content = strings.ReplaceAll(content, "{{appTitleCamel}}", data.AppTitleCamel)

	return content, nil
}

func executeGoModInit(projectName string) error {
	cmd := exec.Command("go", "mod", "init", projectName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go mod init failed: %w", err)
	}

	return nil
}

func renameEnvExample() error {
	// rename .env.example to .env
	// test if .env.example exists
	if _, err := os.Stat(".env.example"); os.IsNotExist(err) {
		return fmt.Errorf(".env.example does not exist")
	}

	// rename .env.example to .env
	if err := os.Rename(".env.example", ".env"); err != nil {
		return fmt.Errorf("failed to rename .env.example to .env: %w", err)
	}

	return nil
}

func installGoDependencies() error {
	// use go mod tidy
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go mod tidy failed: %w", err)
	}

	// dependencies := []string{
	// 	"github.com/gofiber/fiber/v2",
	// 	"github.com/gofiber/template/html/v2",
	// 	"github.com/joho/godotenv",
	// }

	// for _, dep := range dependencies {
	// 	fmt.Printf("Installing %s...\n", dep)
	// 	cmd := exec.Command("go", "get", dep)
	// 	cmd.Stdout = os.Stdout
	// 	cmd.Stderr = os.Stderr

	// 	if err := cmd.Run(); err != nil {
	// 		return fmt.Errorf("failed to install %s: %w", dep, err)
	// 	}
	// }

	return nil
}

func installNodeDependencies() error {
	// Check if pnpm is available, fall back to npm
	var cmd *exec.Cmd
	if _, err := exec.LookPath("pnpm"); err == nil {
		fmt.Println("Installing Node.js dependencies with pnpm...")
		cmd = exec.Command("pnpm", "install")
	} else {
		fmt.Println("Installing Node.js dependencies with npm...")
		cmd = exec.Command("npm", "install")
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("npm/pnpm install failed: %w", err)
	}

	return nil
}

func createMainGo(projectName string) error {
	mainGoContent := fmt.Sprintf(`package main

import (
	"log"
	"os"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.SetPrefix("%s: ")
	log.SetOutput(os.Stderr)

	// add additional code here
}

func main() {
	// Your main application logic will go here
}
`, projectName)

	if err := os.WriteFile("main.go", []byte(mainGoContent), 0644); err != nil {
		return fmt.Errorf("failed to write main.go: %w", err)
	}

	return nil
}

func toCamelCase(s string) string {
	words := strings.FieldsFunc(s, func(c rune) bool {
		return c == '-' || c == '_' || c == ' '
	})

	result := ""
	for _, word := range words {
		if len(word) > 0 {
			result += strings.ToUpper(string(word[0])) + strings.ToLower(word[1:])
		}
	}

	return result
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
