package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"log"
	"math"
	"os"
	"os/exec"
	"strconv"

	"github.com/nfnt/resize"
	"github.com/otiai10/copy"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

func main() {
	app := pocketbase.New()

	// serves static files from the provided public dir (if exists)
	app.OnBeforeServe().Add(func(e *core.ServeEvent) error {
		e.Router.GET("/*", apis.StaticDirectoryHandler(os.DirFS("./src"), false))
		return nil
	})

	app.OnRecordAfterCreateRequest("caskers").Add(func(e *core.RecordCreateEvent) error {
		log.Println(e.Record)

		// Fetch the port from the record
		portInterface := e.Record.Get("port")

		// Try to assert as int first
		portValue, ok := portInterface.(int)
		if !ok {
			// If it's not an int, try to assert as float64
			portFloat, isFloat64 := portInterface.(float64)
			if isFloat64 {
				if portFloat != math.Floor(portFloat) {
					return fmt.Errorf("port field as float64 has a decimal part, invalid")
				}
				portValue = int(portFloat)
			} else {
				// If it's not a float64, try to assert it as a string and then convert
				portStr, isString := portInterface.(string)
				if !isString {
					return fmt.Errorf("port field is neither an integer, float64, nor a string, stupid")
				}

				// Convert the string to an integer
				var err error
				portValue, err = strconv.Atoi(portStr)
				if err != nil {
					return fmt.Errorf("failed to convert port string to integer: %v", err)
				}
			}
		}

		// Now portValue contains the port as an integer, proceed with the rest of your logic

		nameInterface := e.Record.Get("name")
		nameValue, ok := nameInterface.(string)
		if !ok {
			return fmt.Errorf("name field is not a string or missing")
		}

		// Set the paths
		defaultPath := "./caskers/default"
		newPath := fmt.Sprintf("./caskers/%s", nameValue)

		// Copy the default directory to the new directory using otiai10/copy
		err := copy.Copy(defaultPath, newPath)
		if err != nil {
			return fmt.Errorf("failed to copy directory: %v", err)
		}
		err = UpdateManifest(nameValue)
		if err != nil {
			log.Printf("Error updating manifest: %v", err)
		}
		// Construct the path to the source icon
		recordIDInterface := e.Record.Get("id")
		recordID, ok := recordIDInterface.(string)
		if !ok {
			return fmt.Errorf("recordID field is not a string or missing")
		}

		iconNameInterface := e.Record.Get("icon")
		iconName, ok := iconNameInterface.(string)
		if !ok {
			return fmt.Errorf("iconName field is not a string or missing")
		}

		sourceIconPath := fmt.Sprintf("./pb_data/storage/wcb5o2t312i8q8r/%s/%s", recordID, iconName)

		// Set the destination icon path
		destIconPath192 := fmt.Sprintf("./caskers/%s/pb_public/icon192.png", nameValue)

		// Copy the icon from the source to the destination, replacing the placeholder
		err = copyFile(sourceIconPath, destIconPath192)
		if err != nil {
			return fmt.Errorf("failed to copy icon: %v", err)
		}
		// Resize the copied icon192 to 192x192
		err = resizeImage(destIconPath192, destIconPath192, 192, 192)
		if err != nil {
			return fmt.Errorf("failed to resize icon192: %v", err)
		}

		// Set the destination icon path for icon512
		destIconPath512 := fmt.Sprintf("./caskers/%s/pb_public/icon512.png", nameValue)

		// Copy the icon512 from the source to the destination, replacing the placeholder
		err = copyFile(sourceIconPath, destIconPath512)
		if err != nil {
			return fmt.Errorf("failed to copy icon512: %v", err)
		}

		// Resize the copied icon512 to 512x512
		err = resizeImage(destIconPath512, destIconPath512, 512, 512)
		if err != nil {
			return fmt.Errorf("failed to resize icon512: %v", err)
		}

		// Generate the systemd file with the fetched port
		err = GenerateSystemdFile(nameValue, portValue)
		if err != nil {
			return fmt.Errorf("failed to generate systemd file: %v", err)
		}
		// After writing the systemd file:
		cmd := exec.Command("systemctl", "enable", fmt.Sprintf("%s.service", nameValue))
		err = cmd.Run()
		if err != nil {
			return fmt.Errorf("failed to enable service: %v", err)
		}

		cmd = exec.Command("systemctl", "start", fmt.Sprintf("%s.service", nameValue))
		err = cmd.Run()
		if err != nil {
			return fmt.Errorf("failed to start service: %v", err)
		}

		// Generate the nginx config with the fetched port
		err = GenerateNginxConfig(nameValue, portValue)
		if err != nil {
			return fmt.Errorf("failed to generate nginx config: %v", err)
		}
		// // After writing the nginx config file:
		// err = CreateNginxSymlink(nameValue)
		// if err != nil {
		// 	return fmt.Errorf("failed to create nginx symlink: %v", err)
		// }

		cmd = exec.Command("systemctl", "restart", "nginx")
		err = cmd.Run()
		if err != nil {
			return fmt.Errorf("failed to restart nginx: %v", err)
		}

		return nil
	})

	app.OnRecordAfterDeleteRequest("caskers").Add(func(e *core.RecordDeleteEvent) error {
		// Assuming the record's name field determines the directory name, like in your previous examples
		nameInterface := e.Record.Get("name")
		nameValue, ok := nameInterface.(string)
		if !ok {
			return fmt.Errorf("name field is not a string or missing")
		}

		// Set the directory path
		dirPath := fmt.Sprintf("./caskers/%s", nameValue)

		// Delete the directory
		err := os.RemoveAll(dirPath)
		if err != nil {
			return fmt.Errorf("failed to delete directory: %v", err)
		}
		// Before removing systemd service:
		cmd := exec.Command("systemctl", "disable", fmt.Sprintf("%s.service", nameValue))
		err = cmd.Run()
		if err != nil {
			return fmt.Errorf("failed to disable service: %v", err)
		}

		cmd = exec.Command("systemctl", "stop", fmt.Sprintf("%s.service", nameValue))
		err = cmd.Run()
		if err != nil {
			return fmt.Errorf("failed to stop service: %v", err)
		}
		// Before removing nginx config:
		symlinkPath := fmt.Sprintf("/etc/nginx/sites-partials/%s.conf", nameValue)
		err = os.Remove(symlinkPath)
		if err != nil {
			return fmt.Errorf("failed to remove nginx symlink: %v", err)
		}
		err = RemoveServiceAndConfig(nameValue)
		if err != nil {
			return fmt.Errorf("failed to remove nginx and system config: %v", err)
		}

		return nil
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}

// CreateNginxSymlink creates a symlink in sites-enabled for the nginx configuration
// func CreateNginxSymlink(nameValue string) error {
// 	src := fmt.Sprintf("/etc/nginx/sites-available/%s", nameValue)
// 	dst := fmt.Sprintf("/etc/nginx/sites-enabled/%s", nameValue)
// 	return os.Symlink(src, dst)
// }

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}

	err = destFile.Sync()
	return err
}

type Icon struct {
	Src   string `json:"src"`
	Sizes string `json:"sizes"`
	Type  string `json:"type"`
}

type Screenshot struct {
	Src        string `json:"src"`
	Type       string `json:"type"`
	Sizes      string `json:"sizes"`
	FormFactor string `json:"form_factor"`
}

type Manifest struct {
	Name            string        `json:"name"`
	ShortName       string        `json:"short_name"`
	Categories      string        `json:"categories"`
	Description     string        `json:"description"`
	StartURL        string        `json:"start_url"`
	ID              string        `json:"id"`
	Display         string        `json:"display"`
	BackgroundColor string        `json:"background_color"`
	ThemeColor      string        `json:"theme_color"`
	Screenshots     []Screenshot  `json:"screenshots,omitempty"` // Added field, omitempty if not required
	Icons           []Icon        `json:"icons"`
	LaunchHandler   LaunchHandler `json:"launch_handler"`
}

type LaunchHandler struct {
	ClientMode []string `json:"client_mode"` // Updated to handle an array of strings
}

func UpdateManifest(nameValue string) error {
	// Set path for manifest.json
	manifestPath := fmt.Sprintf("./caskers/%s/pb_public/manifest.json", nameValue)

	// Read the file
	fileBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to read manifest.json: %v", err)
	}

	// Unmarshal the JSON content into the Manifest struct
	var manifest Manifest
	err = json.Unmarshal(fileBytes, &manifest)
	if err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %v", err)
	}

	// Update only the name and short_name
	manifest.Name = nameValue
	manifest.ShortName = nameValue
	manifest.ID = "/" + nameValue

	// Marshal the struct back to JSON
	updatedBytes, err := json.MarshalIndent(manifest, "", "  ") // Indented for prettier output
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %v", err)
	}

	// Write the updated content back to the file
	err = os.WriteFile(manifestPath, updatedBytes, 0644)
	if err != nil {
		return fmt.Errorf("failed to write updated manifest.json: %v", err)
	}

	return nil
}

// GenerateSystemdFile creates a systemd service file for the service
func GenerateSystemdFile(nameValue string, port int) error {
	serviceContent := fmt.Sprintf(`[Unit]
	Description=%s

	[Service]
	Type=simple
	User=root
	Group=root
	WorkingDirectory=/var/www/casker/caskers/%[1]s
	LimitNOFILE=4096
	Restart=always
	RestartSec=5s
	StandardOutput=append:/var/www/casker/caskers/%[1]s/errors.log
	StandardError=append:/var/www/casker/caskers/%[1]s/errors.log
	ExecStart=/var/www/casker/caskers/%[1]s/myapp serve --http="127.0.0.1:%d"
	
	[Install]
	WantedBy=multi-user.target
	`, nameValue, port)

	// Define the path for the systemd service file
	path := fmt.Sprintf("/lib/systemd/system/%s.service", nameValue)

	// Write the service file content to the file system
	return os.WriteFile(path, []byte(serviceContent), 0644)
}

// GenerateNginxConfig creates an nginx config block for the service
func GenerateNginxConfig(nameValue string, port int) error {
	configContent := fmt.Sprintf(`
		location /%s/ {
			proxy_set_header Connection '';
			proxy_http_version 1.1;
			proxy_read_timeout 360s;
	
			proxy_set_header Host $host;
			proxy_set_header X-Real-IP $remote_addr;
			proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
			proxy_set_header X-Forwarded-Proto $scheme;
	
			rewrite ^/%[1]s/(.*)$ /$1 break;
			
			proxy_pass http://127.0.0.1:%d/;
		}
	`, nameValue, port)

	path := fmt.Sprintf("/etc/nginx/sites-partials/%s.conf", nameValue)
	return os.WriteFile(path, []byte(configContent), 0644)
}

// RemoveServiceAndConfig deletes the systemd service and nginx config
func RemoveServiceAndConfig(nameValue string) error {
	// Remove systemd service
	err := os.Remove(fmt.Sprintf("/lib/systemd/system/%s.service", nameValue))
	if err != nil {
		return err
	}

	// Remove nginx config
	err = os.Remove(fmt.Sprintf("/etc/nginx/sites-partials/%s.conf", nameValue))
	if err != nil {
		return err
	}

	// Reloading nginx
	cmd := exec.Command("systemctl", "reload", "nginx")
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to reload nginx: %v", err)
	}

	return nil
}

func resizeImage(sourcePath, destPath string, newWidth, newHeight uint) error {
	// Open the file
	file, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %v", err)
	}
	defer file.Close()

	// Decode the image
	img, _, err := image.Decode(file)
	if err != nil {
		return fmt.Errorf("failed to decode image: %v", err)
	}

	// Resize the image
	resizedImg := resize.Resize(newWidth, newHeight, img, resize.Lanczos3)

	// Create the output file
	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %v", err)
	}
	defer out.Close()

	// Write the resized image to the output file
	err = png.Encode(out, resizedImg)
	if err != nil {
		return fmt.Errorf("failed to encode resized image: %v", err)
	}

	return nil
}
