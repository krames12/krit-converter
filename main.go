package main

import (
	"archive/zip"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// FileData holds information about each file
type FileData struct {
	Name string
	Path string
}

// PageData holds data to render HTML templates
type PageData struct {
	Title   string
	Message string
	Files   []FileData
	ZipPath string
	ZipName string
}

var templates = template.Must(template.ParseGlob("templates/*.html"))

// Predefined array of strings to be converted
var textsToConvert = []string{
	"0",
	"1",
	"2",
	"3",
	"4",
	"5",
	"6",
	"7",
	"8",
	"9",
	"10",
	"11",
	"12",
	"13",
	"14",
	"15",
	"16",
	"17",
	"18",
	"19",
	"20",
	"30",
	"40",
	"50",
	"60",
	"70",
	"80",
	"90",
	"00",
	"6.",
	"6_",
	"9.",
	"9_",
}

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

	if !isValidFontFile(handler.Filename) {
		http.Error(w, "Invalid file format. Only .ttf and .otf are allowed.", http.StatusBadRequest)
		return
	}

	tempDirUUID := uuid.New().String()
	tempDir := filepath.Join("uploads", tempDirUUID)
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

	files := []FileData{}
	for _, text := range textsToConvert {
		svgFilePath, err := generateSVG(tempFilePath, tempDir, text)
		if err != nil {
			http.Error(w, "Error generating SVG", http.StatusInternalServerError)
			return
		}
		files = append(files, FileData{
			Name: filepath.Base(svgFilePath),
			Path: svgFilePath,
		})
	}

	_, err = createZipFile(tempDir, handler.Filename, files)
	if err != nil {
		http.Error(w, "Error creating ZIP file", http.StatusInternalServerError)
		return
	}

	cleanupTempFiles(tempDir, ".zip")

	go scheduleCleanup(tempDir, 1*time.Hour)

	http.Redirect(w, r, fmt.Sprintf("/result/%s", tempDirUUID), http.StatusSeeOther)
}

func resultHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/result/")
	if id == "" {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	tempDir := filepath.Join("uploads", id)
	if _, err := os.Stat(tempDir); os.IsNotExist(err) {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	files, err := getFiles(tempDir)
	if err != nil {
		http.Error(w, "Error reading files", http.StatusInternalServerError)
		return
	}

	zipFilePath, zipFileName := "", ""
	for _, file := range files {
		if strings.HasSuffix(file.Path, ".zip") {
			zipFilePath = "/" + file.Path
			zipFileName = file.Name
			break
		}
	}

	renderTemplate(w, "success", PageData{
		Title:   "Upload Successful",
		Message: "Your SVG files have been created and compressed.",
		Files:   files,
		ZipPath: zipFilePath,
		ZipName: zipFileName,
	})
}

func isValidFontFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".ttf" || ext == ".otf"
}

func generateSVG(inputPath, outputDir, text string) (string, error) {
	// Use the text as the base name for the files, sanitized to ensure it's a valid file name
	baseName := sanitizeFileName(text)
	bitmapPath := filepath.Join(outputDir, baseName+".bmp")
	svgPath := filepath.Join(outputDir, baseName+".svg")

	// Run ImageMagick to create a bitmap
	cmd := exec.Command("convert", "-size", "100x100", "xc:white", "-font", inputPath, "-pointsize", "72", "-fill", "black", "-draw", fmt.Sprintf("text 10,70 '%s'", text), bitmapPath)
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

func createZipFile(dir, originalFilename string, files []FileData) (string, error) {
	zipFilename := strings.TrimSuffix(originalFilename, filepath.Ext(originalFilename)) + ".zip"
	zipFilePath := filepath.Join(dir, zipFilename)

	zipFile, err := os.Create(zipFilePath)
	if err != nil {
		return "", err
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	for _, file := range files {
		f, err := os.Open(file.Path)
		if err != nil {
			return "", err
		}
		defer f.Close()

		w, err := zipWriter.Create(file.Name)
		if err != nil {
			return "", err
		}

		_, err = io.Copy(w, f)
		if err != nil {
			return "", err
		}
	}

	return filepath.Join(dir, zipFilename), nil
}

func cleanupTempFiles(tempDir, excludeExt string) {
	files, err := os.ReadDir(tempDir)
	if err != nil {
		log.Printf("error reading temp directory: %v", err)
		return
	}
	for _, file := range files {
		filePath := filepath.Join(tempDir, file.Name())
		if !strings.HasSuffix(filePath, excludeExt) {
			os.Remove(filePath)
		}
	}
}

func scheduleCleanup(dir string, delay time.Duration) {
	time.Sleep(delay)
	os.RemoveAll(dir)
}

// sanitizeFileName removes characters that are not allowed in file names
func sanitizeFileName(text string) string {
	sanitized := strings.ReplaceAll(text, " ", "_")
	sanitized = strings.ReplaceAll(sanitized, "/", "_")
	sanitized = strings.ReplaceAll(sanitized, "\\", "_")
	return sanitized
}

func getFiles(dir string) ([]FileData, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var fileData []FileData
	for _, file := range files {
		fileData = append(fileData, FileData{
			Name: file.Name(),
			Path: filepath.Join(dir, file.Name()),
		})
	}
	return fileData, nil
}

func main() {
	http.HandleFunc("/", uploadHandler)
	http.HandleFunc("/result/", resultHandler)
	http.Handle("/uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir("./uploads"))))
	log.Println("Starting server on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
