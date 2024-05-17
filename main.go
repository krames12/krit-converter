package main

import (
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// PageData holds data to render HTML templates
type PageData struct {
	Title   string
	Message string
	Files   []FileData
}

// FileData holds information about each file
type FileData struct {
	Name string
	Path string
}

var templates = template.Must(template.ParseGlob("templates/*.html"))

func renderTemplate(w http.ResponseWriter, tmpl string, data PageData) {
	err := templates.ExecuteTemplate(w, tmpl+".html", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		renderTemplate(w, "index", PageData{Title: "File Upload"})
		return
	}

	r.ParseMultipartForm(10 << 20) // 10 MB limit

	file, handler, err := r.FormFile("font")
	if err != nil {
		http.Error(w, "Error retrieving the file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	tempDir := filepath.Join("uploads", uuid.New().String())
	os.MkdirAll(tempDir, os.ModePerm)

	tempFilePath := filepath.Join(tempDir, handler.Filename)
	tempFile, err := os.Create(tempFilePath)
	if err != nil {
		http.Error(w, "Error creating a temporary file", http.StatusInternalServerError)
		return
	}
	defer tempFile.Close()

	_, err = io.Copy(tempFile, file)
	if err != nil {
		http.Error(w, "Error saving the file", http.StatusInternalServerError)
		return
	}

	// Generate SVG from font using imagemagick and potrace
	svgFilePath, err := generateSVG(tempFilePath, tempDir)
	if err != nil {
		http.Error(w, "Error generating SVG", http.StatusInternalServerError)
		return
	}

	// Cleanup temporary files except for the SVG
	cleanupTempFiles(tempDir, svgFilePath)

	// Create file data
	fileData := FileData{
		Name: filepath.Base(svgFilePath),
		Path: svgFilePath,
	}

	// Render success template with file information
	renderTemplate(w, "success", PageData{
		Title:   "Upload Successful",
		Message: "Your SVG file has been created.",
		Files:   []FileData{fileData},
	})

	// Schedule cleanup of the upload directory
	go scheduleCleanup(tempDir, 1*time.Hour)
}

func generateSVG(inputPath, outputDir string) (string, error) {
	bitmapPath := filepath.Join(outputDir, "output.bmp")
	svgPath := filepath.Join(outputDir, "output.svg")

	// Run ImageMagick to create a bitmap
	cmd := exec.Command("convert", "-size", "100x100", "xc:white", "-font", inputPath, "-pointsize", "72", "-fill", "black", "-draw", "text 10,70 'A'", bitmapPath)
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("error running ImageMagick: %v", err)
	}

	// Run potrace to convert the bitmap to SVG
	cmd = exec.Command("potrace", bitmapPath, "-s", "-o", svgPath)
	err = cmd.Run()
	if err != nil {
		return "", fmt.Errorf("error running potrace: %v", err)
	}

	return svgPath, nil
}

func cleanupTempFiles(tempDir, excludeFile string) {
	files, err := os.ReadDir(tempDir)
	if err != nil {
		log.Printf("error reading temp directory: %v", err)
		return
	}
	for _, file := range files {
		filePath := filepath.Join(tempDir, file.Name())
		if filePath != excludeFile {
			os.Remove(filePath)
		}
	}
}

func scheduleCleanup(dir string, delay time.Duration) {
	time.Sleep(delay)
	os.RemoveAll(dir)
}

func main() {
	http.HandleFunc("/", uploadHandler)
	http.Handle("/uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir("./uploads"))))
	log.Println("Starting server on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
