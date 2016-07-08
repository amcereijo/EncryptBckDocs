package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"

	"github.com/fsnotify/fsnotify"
)

const configFileName = "config.json"
const clientSecretFileName = "client_secret.json"

var appFiles = []string{configFileName, clientSecretFileName, "EncryptBckDocs.go", "EncryptBckDocs"}

var driveSrv *drive.Service // drive service

var configApp appConfig // app configuration object

type appConfig struct {
	FolderName    string   `json:"folderName"`
	LastUpdate    string   `json:"lastUpdate"`
	FolderToWatch []string `json:"folderToWatch"`
}

// getClient uses a Context and Config to retrieve a Token
// then generate a Client. It returns the generated Client.
func getClient(ctx context.Context, config *oauth2.Config) *http.Client {
	cacheFile, err := tokenCacheFile()
	if err != nil {
		log.Fatalf("Unable to get path to cached credential file. %v", err)
	}
	tok, err := tokenFromFile(cacheFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(cacheFile, tok)
	}
	return config.Client(ctx, tok)
}

// getTokenFromWeb uses Config to request a Token.
// It returns the retrieved Token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var code string
	if _, err := fmt.Scan(&code); err != nil {
		log.Fatalf("Unable to read authorization code %v", err)
	}

	tok, err := config.Exchange(oauth2.NoContext, code)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web %v", err)
	}
	return tok
}

// tokenCacheFile generates credential file path/filename.
// It returns the generated credential path/filename.
func tokenCacheFile() (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", err
	}
	tokenCacheDir := filepath.Join(usr.HomeDir, ".credentials")
	os.MkdirAll(tokenCacheDir, 0700)
	return filepath.Join(tokenCacheDir,
		url.QueryEscape("EncryptBckDocs.json")), err
}

// tokenFromFile retrieves a Token from a given file path.
// It returns the retrieved Token and any read error encountered.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	t := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(t)
	defer f.Close()
	return t, err
}

// saveToken uses a file path to create a file and store the
// token in it.
func saveToken(file string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", file)
	f, err := os.Create(file)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

func findHolderFolder(folderName string) (file *drive.File, err error) {
	r, err := driveSrv.Files.List().Q("mimeType='application/vnd.google-apps.folder' and explicitlyTrashed=false").Fields("nextPageToken, files(id, name, mimeType)").Do()
	if err != nil {
		return nil, err
	}
	var folder *drive.File
	if len(r.Files) > 0 {
		for _, actualFile := range r.Files {
			// fmt.Printf("-- NAME: %s - ID: (%s) - TYPE:%s\n", actualFile.Name, actualFile.Id, actualFile.MimeType)
			if actualFile.Name == folderName {
				folder = actualFile
				//	break
			}
		}
		if folder == nil {
			errorString := fmt.Sprintf("No folder with name \"%s\"", folderName)
			err = errors.New(errorString)
		}
	} else {
		err = errors.New("No folders")
	}
	return folder, err
}

func findUploadFileInDrive(fileName string, parentID string) (fileToUpload *drive.File, err error) {
	log.Println("findUploadFileInDrive: ", fileName)
	r, err := driveSrv.Files.List().Q("'" + parentID + "' in parents and explicitlyTrashed=false and name='" + fileName + "'").Fields("files(id, name)").Do()
	if err != nil {
		return nil, err
	}
	if len(r.Files) > 0 {
		fileToUpload = r.Files[0]
	}
	return fileToUpload, err
}

func updateLastUpdateAppConfig() {
	configApp.LastUpdate = time.Now().String()
	saveConfigJSONFile()
}

func updateFileInDrive(driveFileToUpload *drive.File, goFile *os.File) (err error) {
	fmt.Printf("Upate existing file %s\n!!", driveFileToUpload.Name)
	driveFileToUpdate := &drive.File{
		Name: filepath.Base(driveFileToUpload.Name),
	}

	_, err = driveSrv.Files.Update(driveFileToUpload.Id, driveFileToUpdate).Media(goFile).Do()
	if err != nil {
		panic(err)
	} else {
		fmt.Printf("Updated file \"%s\"!!\n", driveFileToUpload.Name)
		updateLastUpdateAppConfig()
	}

	return err
}

func uploadNewFileToDrive(folderFile *drive.File, fileToUploadName string, fileToUploadURL string, goFile *os.File) (err error) {
	parents := []string{folderFile.Id}
	driveFileToUpload := &drive.File{
		Parents: parents,
		Name:    filepath.Base(fileToUploadName),
	}
	_, err = driveSrv.Files.Create(driveFileToUpload).Media(goFile).Do()
	if err != nil {
		panic(err)
	} else {
		fmt.Printf("Uploaded file \"%s\" to \"%s\" !!\n", fileToUploadName, folderFile.Name)
		updateLastUpdateAppConfig()
	}
	return err
}

func loadConfig() (config appConfig, err error) {
	configFileContent, err := os.Open(configFileName)
	if err != nil {
		return config, err
	}
	defer configFileContent.Close()

	jsonParser := json.NewDecoder(configFileContent)
	err = jsonParser.Decode(&config)
	if err != nil {
		return config, err
	}

	return config, nil
}

func saveConfigJSONFile() {
	//save json file
	jsonContent, err := json.Marshal(configApp)
	if err != nil {
		log.Printf("ERROR! Cannot create config file: %v ", err)
	} else {
		ioutil.WriteFile(configFileName, jsonContent, 0644)
	}
}

func createConfig() (config appConfig) {
	// create config file
	folderName := "EncryptBckDoc"
	var inputFolderName string
	fmt.Print("Name for the folder to save files (default: EncryptBckDoc): ")
	fmt.Scanln(&inputFolderName)
	if inputFolderName != "" {
		folderName = inputFolderName
	}
	configApp = appConfig{
		FolderName: folderName,
	}
	//save json file
	saveConfigJSONFile()

	return configApp
}

func createFolderInDrive(folderName string) (folderFile *drive.File, err error) {
	log.Printf("Error finding %s : %v\n", folderName, err)
	// create folder
	fileMeta := &drive.File{
		Name:     folderName,
		MimeType: "application/vnd.google-apps.folder",
	}
	folderFile, err = driveSrv.Files.Create(fileMeta).Do()

	return folderFile, err
}

func isNotAppFile(fileName string) (isIt bool) {
	for _, name := range appFiles {
		indexOfName := strings.LastIndex(fileName, name)
		nameLength := len(name)
		fileNameLength := len(fileName)
		if fileNameLength-nameLength == indexOfName {
			return false
		}
	}
	return true
}

func isNotHiddenFile(fileName string) (isHidden bool) {
	return strings.Index(fileName, "/.") != -1
}

func runWatcher(parentFolder *drive.File) {

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	done := make(chan bool)
	go func() {
		for {
			select {
			case event := <-watcher.Events:
				if event.Op&fsnotify.Write == fsnotify.Write {
					if isNotAppFile(event.Name) && !isNotHiddenFile(event.Name) {
						//onlyFileName := strings.Replace(event.Name, actualFileToWatch+"/", "", -1)
						lastPos := strings.LastIndex(event.Name, string(os.PathSeparator))
						actualFileToWatch := event.Name[0:lastPos]
						onlyFileName := event.Name[(lastPos + 1):len(event.Name)]
						log.Println("ToReplace: ", actualFileToWatch+string(os.PathSeparator), " - name: ", event.Name, "  onlyFileName: ", onlyFileName)
						processUpload(event.Name, onlyFileName, parentFolder)
					}
				}
			case err := <-watcher.Errors:
				log.Println("error:", err)
			}
		}
	}()

	for _, actualFileToWatch := range configApp.FolderToWatch {
		log.Println("add to watch: ", actualFileToWatch)
		err = watcher.Add(actualFileToWatch)
		if err != nil {
			log.Fatal(err)
		}
	}

	<-done

}

func uploadActualFilesInWatchDir(parentFolder *drive.File) {
	for _, actualFolderToWatch := range configApp.FolderToWatch {
		log.Println("-uploadActualFilesInWatchDir: ", actualFolderToWatch)
		files, err := ioutil.ReadDir(actualFolderToWatch)
		if err != nil {
			log.Println("Error uploadActualFilesInWatchDir: ", err)
		} else {
			for _, actualFile := range files {
				if !actualFile.IsDir() {
					totalName := actualFolderToWatch + "/" + actualFile.Name()
					if isNotAppFile(totalName) && !isNotHiddenFile(totalName) {
						processUpload(totalName, actualFile.Name(), parentFolder)
					}
				}
			}
		}
	}
}

func processUpload(uploadFilePath string, uploadFileName string, parentFolder *drive.File) {
	goFile, err := os.Open(uploadFilePath)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}

	var driveFileToUpload *drive.File
	driveFileToUpload, err = findUploadFileInDrive(uploadFileName, parentFolder.Id)
	if err != nil {
		log.Fatalf("Error checking if file \"%s\" already exists", uploadFileName)
	}

	if driveFileToUpload != nil {
		log.Println("Update existing file to Drive")
		updateFileInDrive(driveFileToUpload, goFile)
	} else {
		log.Println("Update new file to Drive")
		uploadNewFileToDrive(parentFolder, uploadFileName, uploadFilePath, goFile)
	}
}

func configFolderToWatch() {
	if len(configApp.FolderToWatch) == 0 || configApp.FolderToWatch[0] == "" {
		var folderToWatch string
		fmt.Print("Path to watch (default-actual folder: \".\" ): ")
		fmt.Scanln(&folderToWatch)
		if folderToWatch == "" {
			folderToWatch = "."
		}
		//save in config file
		folderToWatch, _ = filepath.Abs(filepath.Dir(folderToWatch))
		configApp.FolderToWatch = []string{folderToWatch}
		saveConfigJSONFile()
	}
}

func addFolderToWatch() {
	if len(configApp.FolderToWatch) == 0 {
		log.Println("Lauch config option first!!")
	} else {
		var folderToWatch string
		fmt.Print("Path to watch (default-actual folder: \".\" ): ")
		fmt.Scanln(&folderToWatch)
		if folderToWatch == "" {
			folderToWatch = "."
		}

		//save in config file
		folderToWatch, _ = filepath.Abs(filepath.Dir(folderToWatch + "/"))

		isFolderInConfig := false
		for _, actualFoldertoWatch := range configApp.FolderToWatch {
			if actualFoldertoWatch == folderToWatch {
				isFolderInConfig = true
			}
		}
		if isFolderInConfig {
			log.Println("ERROR!! - The folder ir already in config!")
		} else {
			configApp.FolderToWatch = append(configApp.FolderToWatch, folderToWatch)
			saveConfigJSONFile()
		}
	}
}

func showAppConfig() {
	fmt.Printf("\n### Actual configuration ####\n")
	fmt.Printf("###  - Destination folder in Drive: %s\n", configApp.FolderName)
	fmt.Printf("###  - Last syncronization time: %s\n", configApp.LastUpdate)
	fmt.Printf("###  - Local watching folder: %s\n", configApp.FolderToWatch)
	fmt.Printf("### #################### ####\n\n")
}

func runOption(userOption string, backToMenu bool) {
	if userOption == "e" {
		executeApp()
	} else if userOption == "x" {
		os.Exit(0)
	} else if userOption == "c" {
		configApp = createConfig()
		configFolderToWatch()

		if backToMenu {
			showAppMenu()
		}
	} else if userOption == "a" {
		addFolderToWatch()
		if backToMenu {
			showAppMenu()
		}
	} else if userOption == "s" {
		showAppConfig()
		if backToMenu {
			showAppMenu()
		}
	} else {
		log.Fatal("Wrong option: ", userOption)
	}
}

func showAppMenu() {
	optionsWithAppConfig := fmt.Sprintf("Options(case insensitive):\n" +
		"  c - Configure (remove previous configuration)\n" +
		"  s - Show Configuration\n" +
		"  a - Add path to listen\n" +
		"  e - Execute\n" +
		"  x - Exit\n")
	optionsWithoutAppConfig := fmt.Sprintf("Options:\n" +
		"  c - Configure\n" +
		"  x - Exit\n")

	if configApp.FolderName != "" {
		fmt.Printf(optionsWithAppConfig)
	} else {
		fmt.Printf(optionsWithoutAppConfig)
	}

	var userOption string
	fmt.Print("Option: ")
	fmt.Scanln(&userOption)
	userOption = strings.ToLower(userOption)

	runOption(userOption, true)
}

func executeApp() {
	fmt.Printf("Looking for folder \"%s\"...\n", configApp.FolderName)

	folderFile, err := findHolderFolder(configApp.FolderName)
	if err != nil {
		folderFile, err = createFolderInDrive(configApp.FolderName)

		if err != nil {
			panic(err)
		} else {
			fmt.Printf("Created folder \"%s\" for files!!\n", configApp.FolderName)
		}
	}

	fmt.Printf("Found folder %s - ID: (%s) - TYPE:%s\n", folderFile.Name, folderFile.Id, folderFile.MimeType)

	configFolderToWatch()

	uploadActualFilesInWatchDir(folderFile)

	runWatcher(folderFile)
}

func main() {
	arguments := os.Args[1:]

	// start config for Drive
	context := context.Background()

	b, err := ioutil.ReadFile("client_secret.json")
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
	}

	// If modifying these scopes, delete your previously saved credentials
	// at ~/.credentials/drive-go-quickstart.json
	config, err := google.ConfigFromJSON(b, drive.DriveScope)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}
	client := getClient(context, config)

	driveSrv, err = drive.New(client)
	if err != nil {
		log.Fatalf("Unable to retrieve drive Client %v", err)
	}

	// end config for Drive

	configApp, err = loadConfig()
	if err != nil {
		//configApp = createConfig()
		fmt.Println("No app config yet")
	}

	fmt.Println(arguments)
	if len(arguments) >= 1 {
		fmt.Println("Execute listen")
		userOption := strings.Replace(arguments[0], "-", "", -1)
		fmt.Println("userOption: ", userOption)
		runOption(userOption, false)
	} else {
		showAppMenu()
	}

}

/*
TODO
	add more folder to watch
*/
