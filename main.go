// secret-probe: a web page that shows which secrets and credentials the
// environment gives to this project. See README.md.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// loadDotEnv sets each KEY=VALUE from path that is not already in the process
// environment, so values from the factory win. It returns the keys it set.
func loadDotEnv(path string) (map[string]bool, error) {
	fromFile := map[string]bool{}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return fromFile, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(strings.TrimPrefix(line, "export "), "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		// An empty value in .env means "not set", so AWS_SESSION_TOKEN= stays unset.
		if _, set := os.LookupEnv(k); set || v == "" {
			continue
		}
		os.Setenv(k, v)
		fromFile[k] = true
	}
	return fromFile, sc.Err()
}

type row struct {
	Name, Value, Detail string
	OK                  bool
}

func secretRow(name string, fromFile map[string]bool) row {
	v, ok := os.LookupEnv(name)
	switch {
	case !ok || v == "":
		return row{name, "missing", "not set in env or .env", false}
	case fromFile[name]:
		return row{name, v, "source: .env", true}
	default:
		return row{name, v, "source: env", true}
	}
}

// getJSON calls url with a bearer token and decodes the JSON body into v.
func getJSON(url, token string, v any) error {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	client := http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", req.URL.Path, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

func githubRow() row {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return row{"GitHub", "missing", "GITHUB_TOKEN is not set", false}
	}
	var u struct{ Login string }
	if err := getJSON("https://api.github.com/user", token, &u); err != nil {
		return row{"GitHub", "error", err.Error(), false}
	}
	return row{"GitHub", u.Login, "login of GITHUB_TOKEN", true}
}

func terraformRow() row {
	token := os.Getenv("TF_TOKEN_app_terraform_io")
	if token == "" {
		return row{"HCP Terraform", "missing", "TF_TOKEN_app_terraform_io is not set", false}
	}
	var acc struct {
		Data struct {
			Attributes struct{ Username string }
		}
	}
	if err := getJSON("https://app.terraform.io/api/v2/account/details", token, &acc); err != nil {
		return row{"HCP Terraform", "error", err.Error(), false}
	}
	detail := fmt.Sprintf("token user; org %q, project %q", os.Getenv("TF_CLOUD_ORGANIZATION"), os.Getenv("TF_CLOUD_PROJECT"))
	return row{"HCP Terraform", acc.Data.Attributes.Username, detail, true}
}

// awsCLI runs the AWS CLI with the credentials of the process environment.
func awsCLI(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "aws", args...).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return "", errors.New(strings.TrimSpace(string(ee.Stderr)))
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// awsCredSource says where the AWS CLI gets its credentials, without values.
func awsCredSource(fromFile map[string]bool) string {
	if os.Getenv("AWS_ACCESS_KEY_ID") != "" {
		src := "env"
		if fromFile["AWS_ACCESS_KEY_ID"] {
			src = ".env"
		}
		if os.Getenv("AWS_SESSION_TOKEN") != "" {
			return "temporary keys from " + src
		}
		return "access keys from " + src
	}
	if p := os.Getenv("AWS_PROFILE"); p != "" {
		return "profile " + p
	}
	return "AWS default chain"
}

func awsIdentityRow(fromFile map[string]bool) row {
	src := "credentials: " + awsCredSource(fromFile)
	out, err := awsCLI("sts", "get-caller-identity", "--output", "json")
	if err != nil {
		return row{"AWS identity", "error", src + "; " + err.Error(), false}
	}
	var id struct{ Account, Arn string }
	if err := json.Unmarshal([]byte(out), &id); err != nil {
		return row{"AWS identity", "error", err.Error(), false}
	}
	return row{"AWS identity", id.Arn, "account " + id.Account + "; " + src, true}
}

func ssmRow() row {
	name := os.Getenv("SSM_PARAM_NAME")
	if name == "" {
		return row{"SSM parameter", "missing", "SSM_PARAM_NAME is not set", false}
	}
	out, err := awsCLI("ssm", "get-parameter", "--name", name, "--query", "Parameter.Value", "--output", "text")
	if err != nil {
		if strings.Contains(err.Error(), "ParameterNotFound") {
			return row{"SSM parameter", "not found", name, false}
		}
		return row{"SSM parameter", "error", err.Error(), false}
	}
	return row{"SSM parameter", out, name, true}
}

var page = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>secret-probe</title>
<style>body{font-family:sans-serif;margin:2rem}td,th{padding:.4rem .8rem;border-bottom:1px solid #ddd;text-align:left}
.ok{color:#176f2c}.bad{color:#b3261e}code{font-size:1.05em}</style></head>
<body><h1>secret-probe</h1>
<table><tr><th>Check</th><th>Status</th><th>Value</th><th>Detail</th></tr>
{{range .}}<tr data-check="{{.Name}}"><td>{{.Name}}</td>
<td class="{{if .OK}}ok{{else}}bad{{end}}">{{if .OK}}OK{{else}}FAIL{{end}}</td>
<td><code>{{.Value}}</code></td><td>{{.Detail}}</td></tr>
{{end}}</table></body></html>`))

func main() {
	fromFile, err := loadDotEnv(".env")
	if err != nil {
		log.Fatalf("read .env: %v", err)
	}
	http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, "ok") })
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		rows := []row{secretRow("PROBE_SECRET", fromFile), githubRow(), terraformRow(), awsIdentityRow(fromFile), ssmRow()}
		if err := page.Execute(w, rows); err != nil {
			log.Print(err)
		}
	})
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("secret-probe on http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
