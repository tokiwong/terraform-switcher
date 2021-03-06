package main

/*
* Version 0.6.0
* Compatible with Mac OS X ONLY
 */

/*** OPERATION WORKFLOW ***/
/*
* 1- Create /usr/local/terraform directory if does not exist
* 2- Download zip file from url to /usr/local/terraform
* 3- Unzip the file to /usr/local/terraform
* 4- Rename the file from `terraform` to `terraform_version`
* 5- Remove the downloaded zip file
* 6- Read the existing symlink for terraform (Check if it's a homebrew symlink)
* 7- Remove that symlink (Check if it's a homebrew symlink)
* 8- Create new symlink to binary  `terraform_version`
 */

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/Masterminds/semver"

	// original hashicorp upstream have broken dependencies, so using fork as workaround
	// TODO: move back to upstream
	"github.com/kiranjthomas/terraform-config-inspect/tfconfig"
	//	"github.com/hashicorp/terraform-config-inspect/tfconfig"

	"github.com/manifoldco/promptui"
	"github.com/pborman/getopt"
	"github.com/spf13/viper"

	lib "github.com/warrensbox/terraform-switcher/lib"
)

const (
	hashiURL     = "https://releases.hashicorp.com/terraform/"
	defaultBin   = "/usr/local/bin/terraform" //default bin installation dir
	tfvFilename  = ".terraform-version"
	rcFilename   = ".tfswitchrc"
	tomlFilename = ".tfswitch.toml"
)

var version = "0.7.0\n"

func main() {

	custBinPath := getopt.StringLong("bin", 'b', defaultBin, "Custom binary path. For example: /Users/username/bin/terraform")
	listAllFlag := getopt.BoolLong("list-all", 'l', "List all versions of terraform - including beta and rc")
	versionFlag := getopt.BoolLong("version", 'v', "Displays the version of tfswitch")
	helpFlag := getopt.BoolLong("help", 'h', "Displays help message")
	_ = versionFlag

	getopt.Parse()
	args := getopt.Args()

	dir, err := os.Getwd()
	if err != nil {
		log.Printf("Failed to get current directory %v\n", err)
		os.Exit(1)
	}

	tfvfile := dir + fmt.Sprintf("/%s", tfvFilename)     //settings for .terraform-version file in current directory (tfenv compatible)
	rcfile := dir + fmt.Sprintf("/%s", rcFilename)       //settings for .tfswitchrc file in current directory (backward compatible purpose)
	configfile := dir + fmt.Sprintf("/%s", tomlFilename) //settings for .tfswitch.toml file in current directory (option to specify bin directory)

	if *versionFlag {
		fmt.Printf("\nVersion: %v\n", version)
	} else if *helpFlag {
		usageMessage()
	} else {
		/* This block checks to see if the tfswitch toml file is provided in the current path.
		 * If the .tfswitch.toml file exist, it has a higher precedence than the .tfswitchrc file
		 * You can specify the custom binary path and the version you desire
		 * If you provide a custom binary path with the -b option, this will override the bin value in the toml file
		 * If you provide a version on the command line, this will override the version value in the toml file
		 */
		if _, err := os.Stat(configfile); err == nil {
			fmt.Printf("Reading configuration from %s\n", tomlFilename)
			tfversion := ""
			binPath := *custBinPath                         //takes the default bin (defaultBin) if user does not specify bin path
			configfileName := lib.GetFileName(tomlFilename) //get the config file
			viper.SetConfigType("toml")
			viper.SetConfigName(configfileName)
			viper.AddConfigPath(dir)

			errs := viper.ReadInConfig() // Find and read the config file
			if errs != nil {
				fmt.Printf("Unable to read %s provided\n", tomlFilename) // Handle errors reading the config file
				fmt.Println(err)
				os.Exit(1) // exit immediately if config file provided but it is unable to read it
			}

			bin := viper.Get("bin")                  // read custom binary location
			if binPath == defaultBin && bin != nil { // if the bin path is the same as the default binary path and if the custom binary is provided in the toml file (use it)
				binPath = os.ExpandEnv(bin.(string))
			}
			version := viper.Get("version") //attempt to get the version if it's provided in the toml

			if len(args) == 1 { //if the version is passed in the command line
				requestedVersion := args[0]
				listAll := true                                     //set list all true - all versions including beta and rc will be displayed
				tflist, _ := lib.GetTFList(hashiURL, listAll)       //get list of versions
				exist := lib.VersionExist(requestedVersion, tflist) //check if version exist before downloading it

				if exist {
					tfversion = requestedVersion // set tfversion = the version needed
				}
			} else if version != nil { // if the required version in the toml file is provided (use it)
				tfversion = version.(string)
			}

			if *listAllFlag { //show all terraform version including betas and RCs
				listAll := true //set list all true - all versions including beta and rc will be displayed
				installOption(listAll, &binPath)
			} else if tfversion == "" { // if no version is provided, show a dropdown of available release versions
				listAll := false //set list all false - only official release will be displayed
				installOption(listAll, &binPath)
			} else {
				if lib.ValidVersionFormat(tfversion) { //check if version is correct
					lib.Install(tfversion, binPath)
				} else {
					fmt.Println("Invalid terraform version format. Format should be #.#.# or #.#.#-@# where # is numbers and @ is word characters. For example, 0.11.7 and 0.11.9-beta1 are valid versions")
					os.Exit(1)
				}
			}
		} else if module, _ := tfconfig.LoadModule(dir); len(module.RequiredCore) >= 1 && len(args) == 0 { //if there is a .tfswitchrc file, and no commmand line arguments
			tfversion := ""

			// we skip duplicated definitions and use only first one
			tfconstraint := module.RequiredCore[0]
			tflist, _ := lib.GetTFList(hashiURL, true)
			fmt.Printf("Reading required version from terraform code, constraint: %s\n", tfconstraint)

			c, err := semver.NewConstraint(tfconstraint)
			if err != nil {
				fmt.Println("Error parsing constraint:", err)
				os.Exit(1)
			}
			vs := make([]*semver.Version, len(tflist))
			for i, r := range tflist {
				v, err := semver.NewVersion(r)
				if err != nil {
					fmt.Printf("Error parsing version: %s", err)
					os.Exit(1)
				}

				vs[i] = v
			}

			sort.Sort(sort.Reverse(semver.Collection(vs)))

			for _, element := range vs {
				// Validate a version against a constraint.
				if c.Check(element) {
					tfversion = string(element.String())
					fmt.Printf("Matched version: %s\n", tfversion)

					if lib.ValidVersionFormat(tfversion) {
						lib.Install(string(tfversion), *custBinPath)
						break
					} else {
						fmt.Println("Invalid terraform version format. Format should be #.#.# or #.#.#-@# where # is numbers and @ is word characters. For example, 0.11.7 and 0.11.9-beta1 are valid versions")
						os.Exit(1)
					}
				}
			}

			fmt.Println("No version found to match constraint.")
			os.Exit(1)

		} else if _, err := os.Stat(rcfile); err == nil && len(args) == 0 { //if there is a .tfswitchrc file, and no commmand line arguments
			fmt.Printf("Reading required terraform version %s \n", rcFilename)

			fileContents, err := ioutil.ReadFile(rcfile)
			if err != nil {
				fmt.Printf("Failed to read %s file. Follow the README.md instructions for setup. https://github.com/warrensbox/terraform-switcher/blob/master/README.md\n", rcFilename)
				fmt.Printf("Error: %s\n", err)
				os.Exit(1)
			}
			tfversion := strings.TrimSuffix(string(fileContents), "\n")

			if lib.ValidVersionFormat(tfversion) { //check if version is correct
				lib.Install(string(tfversion), *custBinPath)
			} else {
				fmt.Println("Invalid terraform version format. Format should be #.#.# or #.#.#-@# where # is numbers and @ is word characters. For example, 0.11.7 and 0.11.9-beta1 are valid versions")
				os.Exit(1)
			}
		} else if _, err := os.Stat(tfvfile); err == nil && len(args) == 0 { //if there is a .terraform-version file, and no command line arguments
			fmt.Printf("Reading required terraform version %s \n", tfvFilename)

			fileContents, err := ioutil.ReadFile(tfvfile)
			if err != nil {
				fmt.Printf("Failed to read %s file. Follow the README.md instructions for setup. https://github.com/warrensbox/terraform-switcher/blob/master/README.md\n", tfvFilename)
				fmt.Printf("Error: %s\n", err)
				os.Exit(1)
			}
			tfversion := strings.TrimSuffix(string(fileContents), "\n")

			if lib.ValidVersionFormat(tfversion) { //check if version is correct
				lib.Install(string(tfversion), *custBinPath)
			} else {
				fmt.Println("Invalid terraform version format. Format should be #.#.# or #.#.#-@# where # is numbers and @ is word characters. For example, 0.11.7 and 0.11.9-beta1 are valid versions")
				os.Exit(1)
			}
		} else if len(args) == 1 { //if tf version is provided in command line
			if lib.ValidVersionFormat(args[0]) {

				requestedVersion := args[0]
				listAll := true                                     //set list all true - all versions including beta and rc will be displayed
				tflist, _ := lib.GetTFList(hashiURL, listAll)       //get list of versions
				exist := lib.VersionExist(requestedVersion, tflist) //check if version exist before downloading it

				if exist {
					lib.Install(requestedVersion, *custBinPath)
				} else {
					fmt.Println("The provided terraform version does not exist. Try `tfswitch -l` to see all available versions.")
				}

			} else {
				fmt.Println("Invalid terraform version format. Format should be #.#.# or #.#.#-@# where # is numbers and @ is word characters. For example, 0.11.7 and 0.11.9-beta1 are valid versions")
				fmt.Println("Args must be a valid terraform version")
				usageMessage()
			}

		} else if *listAllFlag {
			listAll := true //set list all true - all versions including beta and rc will be displayed
			installOption(listAll, custBinPath)

		} else if len(args) == 0 { //if there are no commmand line arguments

			listAll := false //set list all false - only official release will be displayed
			installOption(listAll, custBinPath)

		} else {
			usageMessage()
		}
	}
}

func usageMessage() {
	fmt.Print("\n\n")
	getopt.PrintUsage(os.Stderr)
	fmt.Println("Supply the terraform version as an argument, or choose from a menu")
}

/* installOption : displays & installs tf version */
/* listAll = true - all versions including beta and rc will be displayed */
/* listAll = false - only official stable release are displayed */
func installOption(listAll bool, custBinPath *string) {

	tflist, _ := lib.GetTFList(hashiURL, listAll) //get list of versions
	recentVersions, _ := lib.GetRecentVersions()  //get recent versions from RECENT file
	tflist = append(recentVersions, tflist...)    //append recent versions to the top of the list
	tflist = lib.RemoveDuplicateVersions(tflist)  //remove duplicate version

	/* prompt user to select version of terraform */
	prompt := promptui.Select{
		Label: "Select Terraform version",
		Items: tflist,
	}

	_, tfversion, errPrompt := prompt.Run()
	tfversion = strings.Trim(tfversion, " *recent") //trim versions with the string " *recent" appended

	if errPrompt != nil {
		log.Printf("Prompt failed %v\n", errPrompt)
		os.Exit(1)
	}

	lib.Install(tfversion, *custBinPath)
	os.Exit(0)
}
