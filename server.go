package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
	"time"

	"github.com/google/uuid"
)

// Define a template for the success page
const successPage = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>File Conversion Success</title>
</head>
<body>
    <h2>File Conversion Successful!</h2>
    <p>Your file has been successfully converted. You can download it from the link below:</p>
    <ul>
        <li><a href="{{.}}">{{.}}</a></li>
    </ul>
</body>
</html>
`

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse the multipart form
	err := r.ParseMultipartForm(10 << 20)
	if err != nil {
		http.Error(w, "Unable to parse form", http.StatusBadRequest)
		return
	}

	// Retrieve the form data
	text := r.FormValue("text")
	fontFile, header, err := r.FormFile("font")
	if err != nil {
		http.Error(w, "Error retrieving the font file", http.StatusInternalServerError)
		return
	}
	defer fontFile.Close()

	// Generate a unique ID for the upload
	id := uuid.New().String()
	uploadDir := filepath.Join("uploads", id)
	os.MkdirAll(uploadDir, os.ModePerm)

	// Save the font file to the unique directory
	tempFontFile, err := os.Create(filepath.Join(uploadDir, header.Filename))
	if err != nil {
		http.Error(w, "Error creating the font file", http.StatusInternalServerError)
		return
	}
	defer tempFontFile.Close()

	_, err = io.Copy(tempFontFile, fontFile)
	if err != nil {
		http.Error(w, "Error saving the font file", http.StatusInternalServerError)
		return
	}

	// Create the output BMP file path
	bitmapPath := filepath.Join(uploadDir, text+".bmp")

	// Construct the ImageMagick command to create a bitmap
	convertCmd := exec.Command("convert",
		"-size", "100x100", "xc:white",
		"-font", tempFontFile.Name(),
		"-pointsize", "72",
		"-fill", "black",
		"-draw", fmt.Sprintf("text 10,70 '%s'", text),
		bitmapPath,
	)

	// Run the ImageMagick command
	err = convertCmd.Run()
	if err != nil {
		http.Error(w, "Error creating the bitmap image", http.StatusInternalServerError)
		return
	}

	// Create the output SVG file path
	svgPath := filepath.Join(uploadDir, text+".svg")

	// Construct the Potrace command to convert BMP to SVG
	potraceCmd := exec.Command("potrace", bitmapPath, "-s", "-o", svgPath)

	// Run the Potrace command
	err = potraceCmd.Run()
	if err != nil {
		http.Error(w, "Error converting bitmap to SVG", http.StatusInternalServerError)
		return
	}

	// Clean up all files except for the SVG
	cleanupFilesExceptSVG(uploadDir, svgPath)

	// Serve a success page with the download link for the SVG
	data := "/" + filepath.Join(uploadDir, filepath.Base(svgPath))
	tmpl := template.Must(template.New("success").Parse(successPage))
	tmpl.Execute(w, data)

	// Schedule cleanup for the uploaded files
	go scheduleCleanup(uploadDir)
}

func cleanupFilesExceptSVG(dir, svgPath string) {
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && path != svgPath {
			os.Remove(path)
		}
		return nil
	})
}

func scheduleCleanup(dir string) {
	time.Sleep(1 * time.Hour) // Wait for 1 hour
	os.RemoveAll(dir)         // Delete the directory and its contents
}

func periodicCleanup() {
	for {
		time.Sleep(1 * time.Hour) // Run cleanup every hour
		cleanupOldFiles("uploads", 1*time.Hour)
	}
}

func cleanupOldFiles(dir string, maxAge time.Duration) {
	now := time.Now()

	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			// Skip the uploads directory itself
			if path == dir {
				return nil
			}

			// Check the age of the directory
			if now.Sub(info.ModTime()) > maxAge {
				os.RemoveAll(path)
			}
		}

		return nil
	})
}

func main() {
	go periodicCleanup() // Start the periodic cleanup in a separate goroutine

	http.HandleFunc("/upload", uploadHandler)

	// Serve static files from the current directory
	http.Handle("/", http.FileServer(http.Dir(".")))

	fmt.Println("Server started at :8080")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		panic(err)
	}
}
