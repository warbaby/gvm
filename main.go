package main

import (
	"fmt"
	"github.com/danielkermode/gvm/web"
	"golang.org/x/sys/windows/registry"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	GvmVersion = "2.0.0"
)

var (
	GvmScript     = "gvm"
	GvmTempScript = "gvm$"
)

// callback function for looping over files. If true, breaks the loop.
type callback func(file os.FileInfo, reg *regexp.Regexp, proot string) bool

func main() {
	args := os.Args
	osArch := strings.ToLower(os.Getenv("PROCESSOR_ARCHITECTURE"))
	detail := ""

	//warbaby
	executable, _ := os.Executable()
	exe := ""
	if strings.HasSuffix(executable, ".exe") {
		exe = ".exe"
		GvmScript += ".bat"
		GvmTempScript += ".bat"
	}
	dir := filepath.Dir(executable)
	innerGoName := filepath.Join(dir, "gvm0"+exe)
	batchName := filepath.Join(dir, GvmScript)

	base := filepath.Base(executable)

	if base == "gvm" || base == "gvm.exe" {
		err := os.Remove(innerGoName)
		if err == nil || os.IsNotExist(err) {
			err = os.Rename(executable, innerGoName)
		}
		if err != nil {
			fmt.Printf("Cannot rename %s to %s: %s", executable, innerGoName, err.Error())
			os.Exit(-1)
			return
		}
		s := "@echo running gvm0 from wrapper script...\n" +
			"@gvm0 %*\n" +
			"@IF EXIST \"gvm$.bat\" (\n" +
			"    @call gvm$.bat\n" +
			"    @del gvm$.bat\n" +
			")"
		err = os.WriteFile(batchName, []byte(s), 755)
		if err != nil {
			fmt.Printf("Cannot write file %s: %s", batchName, err.Error())
			os.Exit(-1)
			return
		}
		fmt.Println("Running gvm not gvm0, rename to gvm0 and generate wrapper script...\nDone, please run again!")
		return
	}

	if osArch == "x86" {
		osArch = "386"
	}

	if len(args) < 2 {
		help()
		return
	}
	if len(args) > 2 {
		detail = args[2]
	}
	if len(args) > 3 {
		fmt.Println("Too many args: gvm expects 2 maximum.")
	}

	// Run the appropriate method
	switch args[1] {
	case "arch":
		fmt.Println("System Architecture: " + osArch)
	case "install":
		success := install(detail, osArch)
		if success {
			fmt.Println("Successfully installed Go version " + detail + ".")
			fmt.Println("To use this version, run gvm use " + detail + ". This will also set your GOROOT.")
		}
	case "gvmroot":
		gvmroot(detail)
	case "goroot":
		goroot(detail)
	case "list":
		listGos()
	case "ls":
		listGos()
	case "uninstall":
		uninstall(detail)
	case "use":
		useGo(detail)
	case "version":
		fmt.Println(GvmVersion)
	case "v":
		fmt.Println(GvmVersion)
	default:
		help()
	}
}

func install(version string, arch string) bool {
	fmt.Println("")

	if os.Getenv("GVMROOT") == "" {
		fmt.Println("No GVMROOT set. Set a GVMROOT for go installations with gvm gvmroot <path>.")
		return false
	}
	if version == "" {
		fmt.Println("Version not specified.")
		return false
	}

	return web.Download(version, "windows-"+arch, root())
}

func gvmroot(path string) {
	fmt.Println("")
	if path == "" {
		if root() == "" {
			fmt.Println("No GVMROOT set.")
		} else {
			fmt.Println("GVMROOT: ", root())
		}
		return
	}
	path = filepath.FromSlash(path)
	_ = os.WriteFile(GvmTempScript, []byte(fmt.Sprintf("SET GVMROOT=%s\nSETX GVMROOT %s\n", path, path)), 0755)
	fmt.Println("Set the GVMROOT to " + path + ".")
}

func goroot(path string) {
	fmt.Println("")
	if path == "" {
		if os.Getenv("GOROOT") == "" {
			fmt.Println("No GOROOT set.")
		} else {
			fmt.Println("GOROOT: ", os.Getenv("GOROOT"))
		}
		return
	}
	newRoot := filepath.FromSlash(path)
	newBin := newRoot + string(filepath.Separator) + "bin"
	oldBin := os.Getenv("GOROOT") + string(os.PathSeparator) + "bin"

	pathEnv := updatePath(oldBin, newBin)
	_ = os.WriteFile(GvmTempScript, []byte(fmt.Sprintf("SET GOROOT=%s\nSET PATH=%s\n", newRoot, pathEnv)), 755)
	fmt.Println("Set the GOROOT to " + newRoot + ". Also updated PATH.")
}

func root() string {
	return filepath.Clean(os.Getenv("GVMROOT"))
}

func listGos() {
	if root() == "" {
		fmt.Println("No GVMROOT set. Set a GVMROOT for go installations with gvm gvmroot <path>.")
		return
	}

	fmt.Printf("listing go versions in '%s'\n", root())

	//store all Go versions so we don't list duplicates
	goVers := make(map[string]bool)

	callb := func(f os.FileInfo, validDir *regexp.Regexp, gvmroot string) bool {
		if f.IsDir() && validDir.MatchString(f.Name()) {
			goDir := filepath.Join(gvmroot, f.Name())
			version := getDirVersion(goDir)
			if version == "" {
				return false
			}
			//check if the version already exists (different named dirs with the same go version can exist)
			_, exists := goVers[version]
			if exists {
				return false
			}
			str := ""
			if goDir == os.Getenv("GOROOT") {
				str += "  * " + version[2:]
			} else {
				str += "    " + version[2:]
			}
			if version != f.Name() {
				str += " \t in " + f.Name()
			}
			goVers[version] = true
			fmt.Println(str)
		}
		return false
	}

	loopFiles(callb)
}

func uninstall(unVer string) {
	if root() == "" {
		fmt.Println("No GVMROOT set. Set a GVMROOT for go installations with gvm gvmroot <path>.")
		return
	}
	if unVer == "" {
		fmt.Println("A version to uninstall must be specified.")
		return
	}

	callb := func(f os.FileInfo, validDir *regexp.Regexp, gvmroot string) bool {
		if f.IsDir() && validDir.MatchString(f.Name()) {
			goDir := filepath.Join(gvmroot, f.Name())
			version := getDirVersion(goDir)
			if version == "go"+unVer {
				os.RemoveAll(goDir)
				fmt.Println("Uninstalled Go version " + version[2:] + ".")
				fmt.Println("Note: If this was your GOROOT, you should use other go version with gvm use <version>")
				return true
			}
		}
		return false
	}

	found := loopFiles(callb)
	if !found {
		fmt.Println("Couldn't uninstall Go version " + unVer + ". Check Go versions with gvm list.")
	}
}

func useGo(newVer string) {
	if root() == "" {
		fmt.Println("No GVMROOT set. Set a GVMROOT for go installations with gvm gvmroot <path>.")
		return
	}
	if newVer == "" {
		fmt.Println("A new version must be specified.")
		return
	}
	callb := func(f os.FileInfo, validDir *regexp.Regexp, gvmroot string) bool {
		if f.IsDir() && validDir.MatchString(f.Name()) {
			goDir := filepath.Join(gvmroot, f.Name())
			version := getDirVersion(goDir)
			if version == "go"+newVer {
				newRoot := filepath.FromSlash(goDir)
				newBin := newRoot + string(filepath.Separator) + "bin"
				oldBin := os.Getenv("GOROOT") + string(os.PathSeparator) + "bin"

				pathEnv := updatePath(oldBin, newBin)
				_ = os.WriteFile(GvmTempScript, []byte(fmt.Sprintf("SET GOROOT=%s\nSET PATH=%s\n", newRoot, pathEnv)), 755)

				fmt.Println("Now using Go version " + version[2:] + ". Set GOROOT to " + goDir + ". Also updated PATH.")

				return true
			}
		}
		return false
	}
	found := loopFiles(callb)
	if !found {
		fmt.Println("Couldn't use Go version " + newVer + ". Check Go versions with gvm list.")
	}
}

func loopFiles(fn callback) bool {
	validDir := regexp.MustCompile(`go(\d\.\d\.\d){0,1}`)
	files, _ := ioutil.ReadDir(root())
	fmt.Println("")
	for _, f := range files {
		shouldBreak := fn(f, validDir, root())
		if shouldBreak {
			return true
		}
	}
	return false
}

func updatePath(oldBin, newBin string) string {
	oldInfo, _ := os.Stat(oldBin)
	path := os.Getenv("PATH")
	pvars := strings.Split(path, ";")
	nvars := make([]string, 0, len(pvars))
	for _, pvar := range pvars {
		check, _ := os.Stat(pvar + string(os.PathSeparator) + "bin")
		if !os.SameFile(oldInfo, check) {
			nvars = append(nvars, pvar)
		}
	}
	return newBin + string(os.PathListSeparator) + strings.Join(nvars, ";")
}

func setEnvVar(envVar string, newVal string, envPath string, machine bool) {
	//this sets the environment variable (GOROOT in this case) for either LOCAL_MACHINE or CURRENT_USER.
	//They are set in the registry. both must be set since the GOROOT could be used from either location.
	regplace := registry.CURRENT_USER
	if machine {
		regplace = registry.LOCAL_MACHINE
	}
	key, err := registry.OpenKey(regplace, envPath, registry.ALL_ACCESS)
	if err != nil {
		fmt.Println("error", err)
		return
	}
	defer key.Close()

	err = key.SetStringValue(envVar, newVal)
	if err != nil {
		fmt.Println("error", err)
	}
}

func updatePathVar(envVar string, oldVal string, newVal string, envPath string, machine bool) {
	//this sets the environment variable for either LOCAL_MACHINE or CURRENT_USER.
	//They are set in the registry. both must be set since the GOROOT could be used from either location.
	regplace := registry.CURRENT_USER
	if machine {
		regplace = registry.LOCAL_MACHINE
	}
	key, err := registry.OpenKey(regplace, envPath, registry.ALL_ACCESS)
	if err != nil {
		fmt.Println("error", err)
		return
	}
	defer key.Close()

	val, _, kerr := key.GetStringValue(envVar)
	if kerr != nil {
		fmt.Println("error", err)
		return
	}
	pvars := strings.Split(val, ";")
	for i, pvar := range pvars {
		if pvar == newVal+"\\bin" {
			//the requested new value already exists in PATH, do nothing
			return
		}
		if pvar == oldVal+"\\bin" {
			pvars = append(pvars[:i], pvars[i+1:]...)
		}
	}
	val = strings.Join(pvars, ";")
	val = val + ";" + newVal + "\\bin"
	err = key.SetStringValue("PATH", val)
	if err != nil {
		fmt.Println("error", err)
	}
}

func getDirVersion(dir string) string {
	files, _ := os.ReadDir(dir)
	for _, f := range files {
		if f.Name() == "VERSION" {
			dat, err := os.ReadFile(filepath.Join(dir, f.Name()))
			if err != nil {
				return "Error reading file."
			}
			split := strings.Split(string(dat), "\n")
			if len(split) >= 0 {
				return split[0]
			} else {
				return ""
			}
		}
	}
	return ""
}

func help() {
	fmt.Println("\nRunning version " + GvmVersion + ".")
	fmt.Println("\nUsage:")
	fmt.Println(" ")
	fmt.Println("  gvm arch                     : Show architecture of OS.")
	fmt.Println("  gvm install <version>        : The version must be a version of Go.")
	fmt.Println("  gvm gvmroot [path]           : Sets GVMROOT environment variable, also update windows Registry.")
	fmt.Println("  gvm goroot [path]            : Sets/appends GOROOT/PATH. Without the extra arg just shows current GOROOT.")
	fmt.Println("  gvm list                     : List the Go installations at or adjacent to GOROOT. Aliased as ls.")
	fmt.Println("  gvm uninstall <version>      : Uninstall specified version of Go. If it was your GOROOT/PATH, make sure to set a new one after.")
	fmt.Println("  gvm use <version>            : Switch to use the specified version. This will set your GOROOT and PATH.")
	fmt.Println("  gvm version                  : Displays the current running version of gvm for Windows. Aliased as v.")
}
