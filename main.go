package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

type Config struct {
	OriginPath      string   `json:"origin_path"`
	DestinationPath string   `json:"destination_path"`
	FilesToCopy     []string `json:"files_to_copy"`
}

type AppConfig struct {
	CurrentProfile string `json:"current_profile"`
}

var appConfig AppConfig
var config Config
var configFile = "save_manager_config.json"

var saveList *widget.List
var saves []string
var selectedIndex int = -1
var w fyne.Window
var messageLabel *widget.Label

func main() {
	a := app.New()
	w = a.NewWindow("Generic Save Manager")
	w.Resize(fyne.NewSize(550, 400))

	loadConfig()

	messageLabel = widget.NewLabel("") // Reserved space for feedback

	saveList = widget.NewList(
		func() int { return len(saves) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			o.(*widget.Label).SetText(saves[i])
		},
	)

	saveList.OnSelected = func(id widget.ListItemID) {
		selectedIndex = id
	}

	// --- BUTTONS ---
	importBtn := widget.NewButton("Import Save", save)
	loadBtn := widget.NewButton("Load Save", load)
	replaceBtn := widget.NewButton("Replace Save", replace)
	deleteBtn := widget.NewButton("Delete Save", deleteSave)
	renameBtn := widget.NewButton("Rename", rename)
	optionsBtn := widget.NewButton("Options", openOptionsWindow)

	buttonRow := container.NewHBox(importBtn, loadBtn, replaceBtn, deleteBtn, renameBtn, optionsBtn)

	// Responsive layout: scrollable list in center, buttons at bottom
	scrollList := container.NewScroll(saveList)
	mainLayout := container.NewBorder(nil, container.NewVBox(messageLabel, buttonRow), nil, nil, scrollList)

	w.SetContent(mainLayout)
	updateSaves()
	w.ShowAndRun()
}

// ----------------- FUNCTIONS -----------------

func showTemporaryMessage(text string) {
	messageLabel.SetText(text)
	go func() {
		time.Sleep(1 * time.Second)
		messageLabel.SetText("")
	}()
}

func save() {
	saveDir("")
}

func saveDir(dirToUse string) {

	if config.OriginPath == "" || config.DestinationPath == "" {
		dialog.ShowError(fmt.Errorf("origin and destination folders must be set"), w)
		return
	}

	var newDir string
	var newName string
	if len(dirToUse) > 0 && dirToUse != "" {
		// Use provided directory path (for replace)
		newDir = filepath.Join(config.DestinationPath, dirToUse)
	} else {
		// Find the first available save_XXXXXXXX folder name
		used := make(map[string]bool)
		entries, err := os.ReadDir(config.DestinationPath)
		if err != nil {
			dialog.ShowError(err, w)
			return
		}
		for _, e := range entries {
			if e.IsDir() && strings.HasPrefix(e.Name(), "save_") && len(e.Name()) == 13 {
				used[e.Name()] = true
			}
		}

		for i := 0; i <= 99999999; i++ {
			candidate := fmt.Sprintf("save_%08d", i)
			if !used[candidate] {
				newName = candidate
				break
			}
		}
		if newName == "" {
			dialog.ShowError(fmt.Errorf("no available save slot found"), w)
			return
		}
		newDir = filepath.Join(config.DestinationPath, newName)
	}

	if err := os.Mkdir(newDir, 0755); err != nil {
		dialog.ShowError(err, w)
		return
	}

	// Copy only specific files from origin (overwrite if exists)
	for _, filename := range config.FilesToCopy {
		srcPath := filepath.Join(config.OriginPath, filename)
		dstPath := filepath.Join(newDir, filename)

		if _, err := os.Stat(srcPath); os.IsNotExist(err) {
			dialog.ShowError(fmt.Errorf("File "+srcPath+" not found"), w)
			continue
		}

		if err := copyFile(srcPath, dstPath, 0644); err != nil {
			dialog.ShowError(err, w)
			return
		}
	}
	if dirToUse == "" {
		showTemporaryMessage("Save " + newName + " successfully imported ✔️")
	}
	updateSaves()
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return os.Chmod(dst, mode)
}

func load() {
	if selectedIndex < 0 || selectedIndex >= len(saves) {
		dialog.ShowInformation("Load", "Please select a save to load.", w)
		return
	}

	if config.OriginPath == "" || config.DestinationPath == "" {
		dialog.ShowError(fmt.Errorf("Origin and destination folders must be set."), w)
		return
	}

	saveName := saves[selectedIndex]
	savePath := filepath.Join(config.DestinationPath, saveName)

	for _, filename := range config.FilesToCopy {
		srcPath := filepath.Join(savePath, filename)
		dstPath := filepath.Join(config.OriginPath, filename)

		if _, err := os.Stat(srcPath); os.IsNotExist(err) {
			dialog.ShowError(fmt.Errorf("Missing file in save: %s", srcPath), w)
			continue
		}

		if err := copyFile(srcPath, dstPath, 0644); err != nil {
			dialog.ShowError(fmt.Errorf("Failed to restore file: %s", err), w)
			return
		}
	}

	showTemporaryMessage("Save loaded ✓")
}

func replace() {
	if selectedIndex < 0 || selectedIndex >= len(saves) {
		dialog.ShowInformation("Replace", "Please select a save to replace.", w)
		return
	}

	saveName := saves[selectedIndex]
	savePath := filepath.Join(config.DestinationPath, saveName)

	dialog.NewConfirm("Confirm Replace", fmt.Sprintf("Are you sure you want to replace '%s' with current origin save?", saveName), func(confirm bool) {
		if confirm {
			// Delete existing save directory first
			err := os.RemoveAll(savePath)
			if err != nil {
				dialog.ShowError(fmt.Errorf("failed to delete folder: %w", err), w)
				return
			}
			// Then copy origin files into the same directory (reuse the save folder name)
			saveDir(saveName)
			showTemporaryMessage(saveName + " successfully replaced ✔️")
		}
	}, w).Show()

}

func deleteSave() {
	if selectedIndex < 0 || selectedIndex >= len(saves) {
		dialog.ShowInformation("Delete", "Please select a save to delete.", w)
		return
	}

	saveName := saves[selectedIndex]
	savePath := filepath.Join(config.DestinationPath, saveName)

	dialog.NewConfirm("Confirm Delete", fmt.Sprintf("Are you sure you want to delete '%s'?", saveName), func(confirm bool) {
		if confirm {
			err := os.RemoveAll(savePath)
			if err != nil {
				showTemporaryMessage("Failed to delete ❌" + saveName)
				return
			}

			// Clear selection & update list
			selectedIndex = -1
			updateSaves()
			showTemporaryMessage(saveName + " successfully deleted ✔️")
		}
	}, w).Show()
}

func rename() {
	if selectedIndex < 0 || selectedIndex >= len(saves) {
		dialog.ShowInformation("Rename", "Please select a save to rename.", w)
		return
	}

	oldName := saves[selectedIndex]
	oldPath := filepath.Join(config.DestinationPath, oldName)

	entry := widget.NewEntry()
	entry.SetText(oldName)

	dialog.ShowForm("Rename Save", "Rename", "Cancel",
		[]*widget.FormItem{
			widget.NewFormItem("New Name", entry),
		},
		func(confirm bool) {
			if !confirm {
				return
			}

			newName := strings.TrimSpace(entry.Text)
			if newName == "" {
				dialog.ShowError(fmt.Errorf("Name cannot be empty"), w)
				return
			}
			if newName == oldName {
				// No change, just return
				return
			}
			if strings.ContainsAny(newName, `\/:*?"<>|`) {
				dialog.ShowError(fmt.Errorf("Name contains invalid characters"), w)
				return
			}

			newPath := filepath.Join(config.DestinationPath, newName)
			if _, err := os.Stat(newPath); err == nil {
				dialog.ShowError(fmt.Errorf("A save with this name already exists"), w)
				return
			}

			if err := os.Rename(oldPath, newPath); err != nil {
				dialog.ShowError(fmt.Errorf("Failed to rename save: %w", err), w)
				return
			}

			updateSaves()

			// Restore selection to renamed save
			for i, name := range saves {
				if name == newName {
					saveList.Select(i)
					selectedIndex = i
					break
				}
			}
		}, w)
}

var currentProfile = "default"

func getProfileNames() []string {
	files, err := os.ReadDir("profiles")
	if err != nil {
		return []string{}
	}
	var names []string
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".json") {
			name := strings.TrimSuffix(file.Name(), ".json")
			names = append(names, name)
		}
	}
	return names
}

func loadProfile(name string) {
	currentProfile = name
	appConfig.CurrentProfile = name
	saveAppConfig()

	profilePath := filepath.Join("profiles", name+".json")
	if _, err := os.Stat(profilePath); os.IsNotExist(err) {
		config = Config{} // New empty profile
		saveConfig()
	} else {
		f, err := os.Open(profilePath)
		if err != nil {
			fmt.Println("Failed to load profile:", err)
			return
		}
		defer f.Close()
		json.NewDecoder(f).Decode(&config)
	}

	updateSaves()
}

func openOptionsWindow() {
	opts := fyne.CurrentApp().NewWindow("Options")
	opts.Resize(fyne.NewSize(800, 400))

	originLabel := widget.NewLabel("Origin Folder: " + config.OriginPath)
	destLabel := widget.NewLabel("Destination Folder: " + config.DestinationPath)
	filesLabel := widget.NewLabel("Files to Copy:\n" + strings.Join(config.FilesToCopy, "\n"))
	profilesLabel := widget.NewLabel("Profile Options :")

	originBtn := widget.NewButton("Set Origin", func() {
		dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
			if uri != nil {
				previousOriginPath := config.OriginPath
				config.OriginPath = uri.Path()
				originLabel.SetText("Origin Folder: " + config.OriginPath)
				if config.OriginPath != previousOriginPath {
					config.FilesToCopy = nil
					filesLabel.SetText("Files to Copy:\n")
				}
				saveConfig()
			}
		}, opts)
	})

	destBtn := widget.NewButton("Set Destination", func() {
		dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
			if uri != nil {
				config.DestinationPath = uri.Path()
				destLabel.SetText("Destination Folder: " + config.DestinationPath)
				updateSaves()
				saveConfig()
			}
		}, opts)
	})

	filesBtn := widget.NewButton("Select Files to Copy", func() {
		if config.OriginPath == "" {
			dialog.ShowInformation("Error", "Set Origin folder first.", opts)
			return
		}

		entries, err := os.ReadDir(config.OriginPath)
		if err != nil {
			dialog.ShowError(err, opts)
			return
		}

		var checkboxes []*widget.Check
		selected := map[string]bool{}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			cb := widget.NewCheck(name, func(checked bool) {
				selected[name] = checked
			})

			// Pre-select already saved files
			for _, f := range config.FilesToCopy {
				if f == name {
					cb.SetChecked(true)
					selected[name] = true
					break
				}
			}

			checkboxes = append(checkboxes, cb)
		}

		dialogWin := fyne.CurrentApp().NewWindow("Select Files")
		dialogWin.Resize(fyne.NewSize(400, 500))

		saveBtn := widget.NewButton("Confirm Selection", func() {
			var newList []string
			for file, checked := range selected {
				if checked {
					newList = append(newList, file)
				}
			}
			config.FilesToCopy = newList
			saveConfig()
			filesLabel.SetText("Files to Copy:\n" + strings.Join(config.FilesToCopy, "\n"))
			dialogWin.Close()
		})

		checkboxContainer := container.NewVBox()
		for _, cb := range checkboxes {
			checkboxContainer.Add(cb)
		}

		dialogWin.SetContent(container.NewBorder(nil, saveBtn, nil, nil,
			container.NewVScroll(checkboxContainer),
		))
		dialogWin.Show()
	})

	clearFilesBtn := widget.NewButton("Clear File List", func() {
		config.FilesToCopy = nil
		filesLabel.SetText("Files to Copy:\n")
		saveConfig()
	})

	profileNames := getProfileNames()
	profileSelect := widget.NewSelect(profileNames, func(selected string) {
		saveConfig()
		loadProfile(selected)
		updateSaves()

		originLabel.SetText("Origin Folder: " + config.OriginPath)
		destLabel.SetText("Destination Folder: " + config.DestinationPath)
		filesLabel.SetText("Files to Copy:\n" + strings.Join(config.FilesToCopy, "\n"))
	})

	profileSelect.PlaceHolder = "Select Profile"
	profileSelect.SetSelected(currentProfile)

	newProfileBtn := widget.NewButton("New Profile", func() {
		existing := getProfileNames()
		base := "New Profile "
		counter := 1
		var name string

		for {
			name = base + fmt.Sprint(counter)
			alreadyUsed := false
			for _, p := range existing {
				if p == name {
					alreadyUsed = true
					break
				}
			}
			if !alreadyUsed {
				break
			}
			counter++
		}

		currentProfile = name
		config = Config{}
		appConfig.CurrentProfile = name
		saveAppConfig()
		saveConfig()
		profileSelect.Options = getProfileNames()
		profileSelect.SetSelected(name)
		filesLabel.SetText("Files to Copy:\n")
		originLabel.SetText("Origin Folder: ")
		destLabel.SetText("Destination Folder: ")
	})

	renameProfileBtn := widget.NewButton("Rename Profile", func() {
		entry := widget.NewEntry()
		entry.SetText(currentProfile)

		dialog.ShowForm("Rename Profile", "Rename", "Cancel",
			[]*widget.FormItem{widget.NewFormItem("New Name", entry)},
			func(confirm bool) {
				if !confirm {
					return
				}
				newName := strings.TrimSpace(entry.Text)
				if newName == "" || strings.ContainsAny(newName, `\/:*?"<>|`) {
					dialog.ShowError(fmt.Errorf("Invalid name"), opts)
					return
				}
				oldPath := filepath.Join("profiles", currentProfile+".json")
				newPath := filepath.Join("profiles", newName+".json")
				if _, err := os.Stat(newPath); err == nil {
					dialog.ShowError(fmt.Errorf("Profile already exists"), opts)
					return
				}

				if err := os.Rename(oldPath, newPath); err != nil {
					dialog.ShowError(fmt.Errorf("Rename failed: %w", err), opts)
					return
				}

				currentProfile = newName
				appConfig.CurrentProfile = newName
				saveAppConfig()
				saveConfig()
				profileSelect.Options = getProfileNames()
				profileSelect.SetSelected(newName)
			}, opts)
	})

	deleteProfileBtn := widget.NewButton("Delete Profile", func() {
		if len(getProfileNames()) <= 1 {
			dialog.ShowInformation("Error", "You must have at least one profile.", opts)
			return
		}

		dialog.NewConfirm("Delete Profile", "Are you sure you want to delete the profile '"+currentProfile+"'?", func(confirm bool) {
			if !confirm {
				return
			}

			profilePath := filepath.Join("profiles", currentProfile+".json")
			if err := os.Remove(profilePath); err != nil {
				dialog.ShowError(fmt.Errorf("Failed to delete profile: %w", err), opts)
				return
			}

			// Reload profiles and reset to first profile
			files, err := os.ReadDir("profiles")
			if err != nil {
				dialog.ShowError(fmt.Errorf("Failed to read profiles: %w", err), opts)
				return
			}
			profileFile := files[0]
			currentProfile = strings.TrimSuffix(profileFile.Name(), ".json")
			appConfig.CurrentProfile = currentProfile
			saveAppConfig()
			loadProfile(currentProfile)

			profileSelect.Options = getProfileNames()
			profileSelect.SetSelected(currentProfile)

			originLabel.SetText("Origin Folder: " + config.OriginPath)
			destLabel.SetText("Destination Folder: " + config.DestinationPath)
			filesLabel.SetText("Files to Copy:\n" + strings.Join(config.FilesToCopy, "\n"))

		}, opts).Show()
	})

	profileBtns := container.NewHBox(profileSelect, newProfileBtn, renameProfileBtn, deleteProfileBtn)

	opts.SetContent(container.NewVBox(
		originLabel,
		originBtn,
		destLabel,
		destBtn,
		widget.NewSeparator(),
		filesLabel,
		filesBtn,
		clearFilesBtn,
		widget.NewSeparator(),
		profilesLabel,
		profileBtns,
	))

	opts.Show()
}

func updateSaves() {
	if config.DestinationPath == "" {
		return
	}

	// Remember previously selected save name (if any)
	var previouslySelected string
	if selectedIndex >= 0 && selectedIndex < len(saves) {
		previouslySelected = saves[selectedIndex]
	}

	saves = nil
	entries, err := os.ReadDir(config.DestinationPath)
	if err != nil {
		fmt.Println("Failed to read destination folder:", err)
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			saves = append(saves, e.Name())
		}
	}
	sort.Strings(saves)

	// Refresh the list widget UI
	if saveList != nil {
		saveList.Refresh()
	}

	// Try to restore selection to previously selected save
	newSelection := -1
	if previouslySelected != "" {
		for i, name := range saves {
			if name == previouslySelected {
				newSelection = i
				break
			}
		}
	}

	// If previous selection not found, select last save if exists
	if newSelection == -1 && len(saves) > 0 {
		newSelection = len(saves) - 1
	}

	if saveList != nil {
		if newSelection >= 0 {
			saveList.Select(newSelection)
			selectedIndex = newSelection
		} else {
			saveList.UnselectAll()
			selectedIndex = -1
		}
	} else {
		// No UI selection possible yet
		selectedIndex = newSelection
	}

}

func saveConfig() {
	profilePath := filepath.Join("profiles", currentProfile+".json")
	f, err := os.Create(profilePath)
	if err != nil {
		fmt.Println("Failed to save profile:", err)
		return
	}
	defer f.Close()
	json.NewEncoder(f).Encode(config)
}

func loadConfig() {
	// Create profiles dir if needed
	os.MkdirAll("profiles", os.ModePerm)

	// Load or initialize save_manager_config.json
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		appConfig = AppConfig{CurrentProfile: "default"}
		saveAppConfig()
	} else {
		f, err := os.Open(configFile)
		if err == nil {
			json.NewDecoder(f).Decode(&appConfig)
			f.Close()
		}
	}

	currentProfile = appConfig.CurrentProfile
	loadProfile(currentProfile)
}

func saveAppConfig() {
	f, err := os.Create(configFile)
	if err != nil {
		fmt.Println("Failed to write config file:", err)
		return
	}
	defer f.Close()
	json.NewEncoder(f).Encode(appConfig)
}
