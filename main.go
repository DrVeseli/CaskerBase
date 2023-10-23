package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"

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
		destIconPath := fmt.Sprintf("./caskers/%s/pb_public/icon.png", nameValue)

		// Copy the icon from the source to the destination, replacing the placeholder
		err = copyFile(sourceIconPath, destIconPath)
		if err != nil {
			return fmt.Errorf("failed to copy icon: %v", err)
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

		return nil
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}

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

type LaunchHandler struct {
	ClientMode []string `json:"client_mode"`
}

type Manifest struct {
	Name            string        `json:"name"`
	ShortName       string        `json:"short_name"`
	Categories      string        `json:"categories"`
	Description     string        `json:"description"`
	StartURL        string        `json:"start_url"`
	Display         string        `json:"display"`
	BackgroundColor string        `json:"background_color"`
	ThemeColor      string        `json:"theme_color"`
	Icons           []Icon        `json:"icons"`
	LaunchHandler   LaunchHandler `json:"launch_handler"`
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
Description = %s service

[Service]
Type           = simple
User           = root
Group          = root
LimitNOFILE    = 4096
Restart        = always
RestartSec     = 5s
StandardOutput = append:/root/logs/%s.log
StandardError  = append:/root/logs/%s-error.log
ExecStart      = /root/caskers/%s/myapp serve --http="127.0.0.1:%d"

[Install]
WantedBy = multi-user.target
`, nameValue, nameValue, nameValue, nameValue, port)

	path := fmt.Sprintf("/lib/systemd/system/%s.service", nameValue)
	return os.WriteFile(path, []byte(serviceContent), 0644)
}

// GenerateNginxConfig creates an nginx config block for the service
func GenerateNginxConfig(nameValue string, port int) error {
	configContent := fmt.Sprintf(`
server {
    listen 80;
    server_name %s.yourdomain.com; # customize this if you have a different naming scheme
    location / {
        proxy_pass http://127.0.0.1:%d;
        # ... other proxy settings ...
    }
}
`, nameValue, port)

	path := fmt.Sprintf("/etc/nginx/sites-available/%s", nameValue)
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
	err = os.Remove(fmt.Sprintf("/etc/nginx/sites-available/%s", nameValue))
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
