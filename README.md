#EncryptBckDocs#


## Requirements
* Set the GOPATH environment variable to your working directory.
* Turn on the Drive API:
 * Use this wizard to create or select a project in the Google Developers Console and automatically turn on the API. Click Continue, then Go to credentials.
 * At the top of the page, select the OAuth consent screen tab. Select an Email address, enter a Product name if not already set, and click the Save button.
 * Select the Credentials tab, click the Create credentials button and select OAuth client ID.
 * Select the application type Other, enter the name "Drive API Quickstart", and click the Create button.
 * Click OK to dismiss the resulting dialog.
 * Click the file_download (Download JSON) button to the right of the client ID.
 * Move this file to your working directory and rename it client_secret.json.
 
 ## Links
 * https://developers.google.com/drive/v3/web/quickstart/go#step_1_turn_on_the_api_name
 * https://github.com/fsnotify/fsnotify
 
 ## Libs
 * go get -u google.golang.org/api/drive/v3
 * go get -u golang.org/x/oauth2/...
 * go get -u golang.org/x/sys/...
 * go get -u github.com/fsnotify/fsnotify