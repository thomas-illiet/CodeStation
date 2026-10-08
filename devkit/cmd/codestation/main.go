package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"codestation/devkit/internal/packagefile"
	"codestation/devkit/internal/toolchain"
)

const version = "1.0.0"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "codestation:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		usage(os.Stdout)
		return nil
	}
	if args[0] == "version" || args[0] == "--version" {
		fmt.Printf("codestation %s\nOpenTofu 1.13.1\nCodeStation provider 1.0.0\n", version)
		return nil
	}
	if args[0] != "package" {
		return fmt.Errorf("unknown command %q", args[0])
	}
	args = args[1:]
	directory := "."
	if len(args) >= 2 && args[0] == "--chdir" {
		directory = args[1]
		args = args[2:]
	} else if len(args) > 0 && strings.HasPrefix(args[0], "--chdir=") {
		directory = strings.TrimPrefix(args[0], "--chdir=")
		args = args[1:]
	}
	if len(args) == 0 {
		packageUsage(os.Stdout)
		return nil
	}
	switch args[0] {
	case "init":
		return initPackage(args[1:])
	case "plan", "apply", "destroy":
		return passthrough(directory, args[0], args[1:])
	case "upload":
		return upload(directory, args[1:])
	case "delete":
		return remove(args[1:])
	case "help", "--help", "-h":
		packageUsage(os.Stdout)
		return nil
	default:
		return fmt.Errorf("unknown package command %q", args[0])
	}
}

func passthrough(directory, command string, args []string) error {
	runtime, err := toolchain.Prepare()
	if err != nil {
		return err
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return err
	}
	child := exec.Command(runtime.Tofu, append([]string{command}, args...)...)
	child.Dir = absolute
	child.Stdin = os.Stdin
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	child.Env = append(os.Environ(), "TF_CLI_CONFIG_FILE="+runtime.ConfigFile, "CHECKPOINT_DISABLE=1")
	if err := child.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			os.Exit(exit.ExitCode())
		}
		return err
	}
	return nil
}

func initPackage(args []string) error {
	flags := flag.NewFlagSet("package init", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	namespace := flags.String("namespace", "", "package namespace")
	name := flags.String("name", "", "package name")
	packageVersion := flags.String("version", "1.0.0", "package version")
	summary := flags.String("summary", "", "short summary")
	publisher := flags.String("publisher", "", "publisher")
	directory := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		directory, args = args[0], args[1:]
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	if directory == "" && flags.NArg() == 1 {
		directory = flags.Arg(0)
	}
	if directory == "" || flags.NArg() > 1 {
		return errors.New("usage: codestation package init DIRECTORY [options]")
	}
	values := []struct {
		label string
		value *string
	}{{"Namespace", namespace}, {"Name", name}, {"Summary", summary}, {"Publisher", publisher}}
	interactive := isTerminal(os.Stdin)
	reader := bufio.NewReader(os.Stdin)
	for _, item := range values {
		if *item.value == "" && interactive {
			fmt.Printf("%s: ", item.label)
			text, _ := reader.ReadString('\n')
			*item.value = strings.TrimSpace(text)
		}
		if *item.value == "" {
			return fmt.Errorf("--%s is required in non-interactive mode", strings.ToLower(item.label))
		}
	}
	entries, err := os.ReadDir(directory)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if len(entries) > 0 {
		return errors.New("init refuses to overwrite a non-empty directory")
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	yaml := fmt.Sprintf("schemaVersion: 1\nnamespace: %s\nname: %s\nversion: %s\nsummary: %q\ndescription: %q\npublisher: %q\ncategories: [Development Tools]\ntargets: [linux-amd64, linux-arm64, windows-amd64]\ntofuConstraint: \">= 1.8.0\"\nmodule: main.tf\nreadme: README.md\nicon: icon.svg\nsensitiveCapabilities: []\n", *namespace, *name, *packageVersion, *summary, *summary, *publisher)
	mainTF := "terraform {\n  required_version = \">= 1.8.0\"\n  required_providers {\n    codestation = { source = \"codestation.internal/codestation/packages\", version = \"1.0.0\" }\n  }\n}\n\nresource \"codestation_file\" \"example\" {\n  version = \"1.0.0\"\n  destination = \"~/.local/share/" + *name + "/hello.txt\"\n  content = \"hello from " + *name + "\\n\"\n}\n"
	files := map[string]string{"package.yaml": yaml, "main.tf": mainTF, "README.md": "# " + *name + "\n\n" + *summary + "\n", "icon.svg": "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 64 64\"><rect width=\"64\" height=\"64\" rx=\"12\" fill=\"#2563eb\"/><path d=\"M18 32h28M32 18v28\" stroke=\"white\" stroke-width=\"6\"/></svg>\n", ".gitignore": ".terraform/\n*.tfstate\n*.tfstate.*\n*.tfplan\n*.log\n"}
	for path, content := range files {
		if err := os.WriteFile(filepath.Join(directory, path), []byte(content), 0600); err != nil {
			return err
		}
	}
	runtime, err := toolchain.Prepare()
	if err != nil {
		return err
	}
	child := exec.Command(runtime.Tofu, "init", "-backend=false", "-input=false")
	child.Dir = directory
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	child.Env = append(os.Environ(), "TF_CLI_CONFIG_FILE="+runtime.ConfigFile, "CHECKPOINT_DISABLE=1")
	return child.Run()
}

func upload(defaultDirectory string, args []string) error {
	directory := defaultDirectory
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		directory, args = args[0], args[1:]
	}
	flags := flag.NewFlagSet("package upload", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	registry := flags.String("registry", os.Getenv("CODESTATION_REGISTRY_URL"), "registry URL")
	token := flags.String("token", "", "administration token")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 1 {
		return errors.New("upload accepts at most one directory")
	}
	if flags.NArg() == 1 {
		directory = flags.Arg(0)
	}
	secret := chooseToken(*token)
	if *registry == "" || secret == "" {
		return errors.New("--registry and --token (or CODESTATION_ADMIN_TOKEN) are required")
	}
	if err := validateWithTofu(directory); err != nil {
		return err
	}
	bundle, descriptor, err := packagefile.Build(directory)
	if err != nil {
		return err
	}
	endpoint, err := adminURL(*registry, "/v1/admin/packages")
	if err != nil {
		return err
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("package", descriptor.Namespace+"-"+descriptor.Name+"-"+descriptor.Version+".cspkg")
	if err != nil {
		return err
	}
	if _, err = part.Write(bundle); err != nil {
		return err
	}
	if err = writer.Close(); err != nil {
		return err
	}
	request, err := http.NewRequest(http.MethodPost, endpoint, &body)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Authorization", "Bearer "+secret)
	response, err := (&http.Client{Timeout: 5 * time.Minute}).Do(request)
	if err != nil {
		return errors.New("registry upload failed")
	}
	defer response.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode != http.StatusCreated {
		return fmt.Errorf("registry upload failed: %s: %s", response.Status, strings.TrimSpace(string(responseBody)))
	}
	fmt.Printf("Uploaded %s/%s@%s\n", descriptor.Namespace, descriptor.Name, descriptor.Version)
	return nil
}

func validateWithTofu(directory string) error {
	descriptor, err := packagefile.Load(directory)
	if err != nil {
		return err
	}
	if err = packagefile.ValidateSources(directory, descriptor); err != nil {
		return err
	}
	runtime, err := toolchain.Prepare()
	if err != nil {
		return err
	}
	dataDirectory, err := os.MkdirTemp("", "codestation-tofu-data-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dataDirectory)
	for _, args := range [][]string{{"fmt", "-check", "-recursive"}, {"init", "-backend=false", "-input=false", "-lockfile=readonly"}, {"validate"}} {
		child := exec.Command(runtime.Tofu, args...)
		child.Dir = directory
		child.Stdout = os.Stdout
		child.Stderr = os.Stderr
		child.Env = append(os.Environ(), "TF_CLI_CONFIG_FILE="+runtime.ConfigFile, "TF_DATA_DIR="+dataDirectory, "CHECKPOINT_DISABLE=1")
		if err = child.Run(); err != nil {
			return err
		}
	}
	return nil
}

func remove(args []string) error {
	identity := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		identity, args = args[0], args[1:]
	}
	flags := flag.NewFlagSet("package delete", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	registry := flags.String("registry", os.Getenv("CODESTATION_REGISTRY_URL"), "registry URL")
	token := flags.String("token", "", "administration token")
	yes := flags.Bool("yes", false, "skip confirmation")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if identity == "" && flags.NArg() == 1 {
		identity = flags.Arg(0)
	}
	if identity == "" || flags.NArg() > 1 {
		return errors.New("usage: codestation package delete NAMESPACE/NAME@VERSION --registry URL --token TOKEN")
	}
	secret := chooseToken(*token)
	if *registry == "" || secret == "" {
		return errors.New("--registry and --token (or CODESTATION_ADMIN_TOKEN) are required")
	}
	parts := strings.Split(identity, "@")
	nameParts := strings.Split(parts[0], "/")
	if len(parts) != 2 || len(nameParts) != 2 {
		return errors.New("package identity must be NAMESPACE/NAME@VERSION")
	}
	if !*yes {
		if !isTerminal(os.Stdin) {
			return errors.New("--yes is required in non-interactive mode")
		}
		fmt.Printf("Delete %s? [y/N] ", identity)
		answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if strings.ToLower(strings.TrimSpace(answer)) != "y" {
			return nil
		}
	}
	endpoint, err := adminURL(*registry, "/v1/admin/packages/"+url.PathEscape(nameParts[0])+"/"+url.PathEscape(nameParts[1])+"/"+url.PathEscape(parts[1]))
	if err != nil {
		return err
	}
	request, _ := http.NewRequest(http.MethodDelete, endpoint, nil)
	request.Header.Set("Authorization", "Bearer "+secret)
	response, err := (&http.Client{Timeout: time.Minute}).Do(request)
	if err != nil {
		return errors.New("registry deletion failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		return fmt.Errorf("registry deletion failed: %s: %s", response.Status, strings.TrimSpace(string(body)))
	}
	fmt.Println("Deleted", identity)
	return nil
}

func chooseToken(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	return os.Getenv("CODESTATION_ADMIN_TOKEN")
}
func adminURL(base, suffix string) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("registry URL must use HTTP or HTTPS")
	}
	return strings.TrimRight(base, "/") + suffix, nil
}
func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && (info.Mode()&os.ModeCharDevice) != 0
}
func usage(w io.Writer) {
	fmt.Fprintln(w, "Usage: codestation package [--chdir DIRECTORY] COMMAND\n       codestation version\n\nCommands: package, version, help")
}
func packageUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: codestation package [--chdir DIRECTORY] <init|plan|apply|destroy|upload|delete>")
}
