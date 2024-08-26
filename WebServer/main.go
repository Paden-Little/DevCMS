package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
)

type Post struct {
	Title         string
	FileName      string
	Description   string
	DatePublished string `json:"datepublished"`
	Tags          []string
	Content       string `json:"content"`
}

type PostContent struct {
	Metadata map[string]interface{}
	Content  template.HTML
}

const (
	renderingServer = "http://flask-server:5000/posts"
	username        = "admin"
	password        = "password"
)

func main() {
	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/admin", adminHandler)
	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/dashboard", dashboardHandler)
	http.HandleFunc("/post", postContentHandler)

	// HTMX endpoints for creating, editing, and deleting posts
	http.HandleFunc("/admin/create", createPostHandler)
	http.HandleFunc("/admin/upload", uploadFileHandler)     // Add this line
	http.HandleFunc("/admin/update", updateMetadataHandler) // Add this line
	http.HandleFunc("/admin/edit", editPostHandler)
	http.HandleFunc("/admin/delete", deletePostHandler)

	log.Printf("Up and running!")
	log.Fatal(http.ListenAndServe(":8000", nil))
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	tmpl := template.Must(template.ParseFiles("templates/index.html"))
	posts := getAllPosts()

	data := map[string][]Post{
		"Posts": posts,
	}

	tmpl.Execute(w, data)
}

func adminHandler(w http.ResponseWriter, r *http.Request) {
	// Redirect to login page if not authenticated
	if !isAuthenticated(r) {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	// If authenticated, redirect to dashboard
	http.Redirect(w, r, "/dashboard", http.StatusFound)
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		// Check credentials
		if r.FormValue("username") == username && r.FormValue("password") == password {
			// Set a session cookie
			http.SetCookie(w, &http.Cookie{
				Name:  "authenticated",
				Value: "true",
				Path:  "/",
			})
			http.Redirect(w, r, "/dashboard", http.StatusFound)
			return
		}
		// If credentials are wrong, show an error
		http.Redirect(w, r, "/login?error=1", http.StatusFound)
		return
	}

	// Render the login page
	tmpl := template.Must(template.ParseFiles("templates/login.html"))
	tmpl.Execute(w, nil)
}

func dashboardHandler(w http.ResponseWriter, r *http.Request) {
	if !isAuthenticated(r) {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	// Render the dashboard page
	tmpl := template.Must(template.ParseFiles("templates/dashboard.html"))
	tmpl.Execute(w, nil)
}

func postContentHandler(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		http.Error(w, "Missing 'name' query parameter", http.StatusBadRequest)
		return
	}

	// Fetch post content from Flask server
	resp, err := http.Get(renderingServer + "?name=" + name)
	if err != nil {
		http.Error(w, "Failed to fetch post content", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		http.Error(w, "Post not found", http.StatusNotFound)
		return
	}

	var postContent map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&postContent); err != nil {
		http.Error(w, "Failed to decode post content", http.StatusInternalServerError)
		return
	}

	// Prepare the content and metadata
	content := template.HTML(postContent["content"].(string))
	metadata := postContent["metadata"].(map[string]interface{})

	data := map[string]interface{}{
		"Metadata": metadata,
		"Content":  content,
	}

	// Render the content
	tmpl := template.Must(template.ParseFiles("templates/post_content.html"))
	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, "Failed to render template", http.StatusInternalServerError)
	}
}

func uploadFileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)

		file, header, err := r.FormFile("file")
		if err != nil {
			log.Printf("Failed to get file from form: %v", err)
			http.Error(w, "Could not get uploaded file", http.StatusBadRequest)
			return
		}
		defer file.Close()

		part, err := writer.CreateFormFile("file", header.Filename)
		if err != nil {
			http.Error(w, "Could not create form file", http.StatusInternalServerError)
			return
		}

		_, err = io.Copy(part, file)
		if err != nil {
			http.Error(w, "Could not copy file content", http.StatusInternalServerError)
			return
		}

		err = writer.Close()
		if err != nil {
			http.Error(w, "Could not close multipart writer", http.StatusInternalServerError)
			return
		}

		req, err := http.NewRequest("POST", renderingServer, body)
		if err != nil {
			http.Error(w, "Failed to create request", http.StatusInternalServerError)
			return
		}
		req.Header.Set("Content-Type", writer.FormDataContentType())

		uploadResp, err := http.DefaultClient.Do(req)
		if err != nil {
			http.Error(w, "Failed to upload file to the Python server", http.StatusInternalServerError)
			return
		}
		defer uploadResp.Body.Close()

		if uploadResp.StatusCode != http.StatusOK {
			log.Printf("File upload failed with status: %d", uploadResp.StatusCode)
			http.Error(w, "File upload failed", uploadResp.StatusCode)
			return
		}

		filePathBytes, err := io.ReadAll(uploadResp.Body)
		if err != nil {
			http.Error(w, "Failed to read upload response", http.StatusInternalServerError)
			return
		}
		filePath := string(filePathBytes)

		w.Write([]byte(filePath))
	}
}

func updateMetadataHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPatch {
		fileName := r.URL.Query().Get("filePath")
		if fileName == "" {
			http.Error(w, "Filename not provided", http.StatusBadRequest)
			log.Printf("Filename not provided")
			return
		}

		// Read the metadata from the request body
		var metadata map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&metadata)
		if err != nil {
			http.Error(w, "Could not decode metadata", http.StatusBadRequest)
			log.Printf("Could not decode metadata")
			return
		}

		// Convert the metadata to a JSON string
		metadataJSON, err := json.Marshal(metadata)
		if err != nil {
			http.Error(w, "Failed to encode metadata", http.StatusInternalServerError)
			log.Printf("Failed to encode metadata")
			return
		}

		// URL encode the fileName
		encodedFileName := url.QueryEscape(fileName)

		// Construct the URL
		patchURL := fmt.Sprintf("%s?filePath=%s", renderingServer, encodedFileName)

		// Send the metadata to the Python server for updating
		req, err := http.NewRequest(http.MethodPatch, patchURL, bytes.NewReader(metadataJSON))
		if err != nil {
			http.Error(w, "Error creating request: "+err.Error(), http.StatusInternalServerError)
			log.Printf("Error creating request: %v", req)
			return
		}
		req.Header.Set("Content-Type", "application/json")

		patchResp, err := http.DefaultClient.Do(req)
		if err != nil {
			http.Error(w, "Failed to update metadata", http.StatusInternalServerError)
			log.Printf("Error updating metadata: %v", req)
			return
		}
		defer patchResp.Body.Close()

		// Copy the response to the original response writer
		io.Copy(w, patchResp.Body)
	}
}

func createPostHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		// Process the form data and create the post
		// This would typically involve sending the data to the Flask server
		// For simplicity, this example just redirects back to the dashboard
		http.Redirect(w, r, "/dashboard", http.StatusFound)
		return
	}

	// Render the create post form
	tmpl := template.Must(template.ParseFiles("templates/create_post.html"))
	tmpl.Execute(w, nil)
}

func editPostHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		// Process the form data and edit the post
		// This would typically involve sending the data to the Flask server
		// For simplicity, this example just redirects back to the dashboard
		http.Redirect(w, r, "/dashboard", http.StatusFound)
		return
	}

	// Render the edit post form
	tmpl := template.Must(template.ParseFiles("templates/edit_post.html"))
	tmpl.Execute(w, nil)
}

func deletePostHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("deletepost handler" + r.Method)
	if r.Method == http.MethodDelete {
		// Read the filename from the request body
		log.Printf("Deleting post")
		var requestData map[string]string
		err := json.NewDecoder(r.Body).Decode(&requestData)
		if err != nil {
			http.Error(w, "Failed to decode request body", http.StatusBadRequest)
			log.Printf("Failed to decode request body: %v", err)
			return
		}

		// Extract filename from the request data
		fileName, ok := requestData["filename"]
		if !ok || fileName == "" {
			http.Error(w, "Filename is required", http.StatusBadRequest)
			log.Printf("Filename is missing")
			return
		}

		// URL encode the filename
		encodedFileName := url.QueryEscape(fileName)

		// Construct the URL for the DELETE request to the Python server
		deleteURL := fmt.Sprintf("%s?filename=%s", renderingServer, encodedFileName)
		log.Printf("Sending DELETE request to: %s", deleteURL)

		// Create and send the DELETE request to the Python server
		req, err := http.NewRequest(http.MethodDelete, deleteURL, nil)
		if err != nil {
			http.Error(w, "Failed to create request", http.StatusInternalServerError)
			log.Printf("Failed to create DELETE request: %v", err)
			return
		}

		deleteResp, err := http.DefaultClient.Do(req)
		if err != nil {
			http.Error(w, "Failed to send DELETE request", http.StatusInternalServerError)
			log.Printf("Failed to send DELETE request: %v", err)
			return
		}
		defer deleteResp.Body.Close()

		// Check if the request was successful
		log.Printf("DELETE request status code: %d", deleteResp.StatusCode)
		if deleteResp.StatusCode != http.StatusOK {
			http.Error(w, "Failed to delete post", deleteResp.StatusCode)
			log.Printf("Failed to delete post, status code: %d", deleteResp.StatusCode)
			return
		}

		// Optionally, you can read and log the response body if needed
		responseBody, err := io.ReadAll(deleteResp.Body)
		if err != nil {
			http.Error(w, "Failed to read response body", http.StatusInternalServerError)
			log.Printf("Failed to read response body: %v", err)
			return
		}

		// Write the response from the Python server back to the client
		w.Write(responseBody)
	} else {
		// Handle GET request to render the delete post form
		posts := getAllPosts()
		data := map[string][]Post{
			"Posts": posts,
		}

		tmpl := template.Must(template.ParseFiles("templates/delete_post.html"))
		tmpl.Execute(w, data)
	}
}

func getAllPosts() []Post {
	resp, err := http.Get(renderingServer)
	if err != nil {
		log.Fatalf("Failed to send GET request: %v", err)
	}
	defer resp.Body.Close()

	var posts []Post
	err = json.NewDecoder(resp.Body).Decode(&posts)
	if err != nil {
		log.Fatalf("Failed to unmarshal the JSON response: %v", err)
	}

	return posts
}

func isAuthenticated(r *http.Request) bool {
	cookie, err := r.Cookie("authenticated")
	if err != nil || cookie.Value != "true" {
		return false
	}
	return true
}
