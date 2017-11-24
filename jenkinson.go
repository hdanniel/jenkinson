package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/Jeffail/gabs"
	"github.com/go-ini/ini"
	"github.com/ryanuber/columnize"
	"github.com/urfave/cli"
)

var version string
var cfg_profile = "default"

type Jenkins struct {
	Host        string
	Token       string
	User        string
	Passwd      string
	Crumb       string
	CrumbHeader string
}

type JobCollection struct {
	Jobs []Job `json:"jobs"`
}

type Job struct {
	Class string `json:"_class"`
	Name  string `json:"name"`
	Url   string `json:"url"`
	Color string `json:"color"`
}

var cfg_file = CfgGetHome() + "/.jenkinson/credentials"

func CfgGetHome() string {
	current_user, err := user.Current()
	if err != nil {
		panic(err)
	}
	return current_user.HomeDir
}
func (jenkins *Jenkins) ListJobs() ([]Job, error) {
	jenkins_url_jobs := jenkins.Host + "/api/json?pretty=true"
	//fmt.Println(jenkins_url_jobs)
	req, err := http.NewRequest("GET", jenkins_url_jobs, nil)
	req.SetBasicAuth(jenkins.User, jenkins.Token)
	client := &http.Client{}
	resp, err := client.Do(req)
	//fmt.Println("response Status:", resp.StatusCode)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, nil
	}
	var job_collection JobCollection
	//var console_collection []ConsoleJson
	body, err := ioutil.ReadAll(resp.Body)
	//fmt.Println(string(body))

	err = json.Unmarshal(body, &job_collection)
	if err != nil {
		//fmt.Printf("error: %#v\n", err)
		fmt.Printf("error: %+v\n", err)
		return nil, err
	}
	_ = json.NewDecoder(resp.Body).Decode(&job_collection)
	//fmt.Printf("%#v\n", job_collection.Jobs[0].Name)
	//fmt.Println(string(body))

	return job_collection.Jobs, nil
}

//GetCrumb connects to CrumbIssuer
func GetCrumb(JenkinsHost string, JenkinsUser string, JenkinsToken string) (string, error) {
	JenkinsURLCrumbIssuer := JenkinsHost + "/crumbIssuer/api/xml?xpath=concat(//crumbRequestField,\":\",//crumb)"
	req, err := http.NewRequest("GET", JenkinsURLCrumbIssuer, nil)
	req.SetBasicAuth(JenkinsUser, JenkinsToken)
	client := &http.Client{}
	resp, err := client.Do(req)
	fmt.Println("response Status:", resp.StatusCode)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	//the crumbIssuer is only available if you have enabled "Prevent Cross Site Request Forgery exploits" in the Jenkins configuration.
	if resp.StatusCode == 404 {
		return "Jenkins-Crumb:", nil
	}
	if resp.StatusCode != 200 {
		return "", nil
	}

	body, err := ioutil.ReadAll(resp.Body)
	return string(body), nil
}

func (jenkins *Jenkins) BuildJob(job_name string) (string, error) {
	jenkins_url_build := jenkins.Host + "/job/" + job_name + "/build"
	req, err := http.NewRequest("POST", jenkins_url_build, nil)
	req.Header.Set(jenkins.CrumbHeader, jenkins.Crumb)
	req.SetBasicAuth(jenkins.User, jenkins.Token)
	client := &http.Client{}
	resp, err := client.Do(req)
	fmt.Println("response Status:", resp.StatusCode)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		return "failed", nil
	}
	//body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("%+v\n", err)
		return "", err
	}

	//fmt.Printf("%#v\n", job_collection.Jobs[0].Name)
	//fmt.Printf("%+v\n", resp.Location)
	build_url, err := jenkins.GetJob(strings.Join(resp.Header["Location"], ""))
	if err != nil {
		fmt.Printf("%+v\n", err)
		return "", err
	}
	fmt.Println(build_url)
	return "created", nil
}

func (jenkins *Jenkins) GetLog(job_name string, job_build string) error {
	text_size := "0"
	more_data := "true"
	body := ""
	for more_data == "true" {
		body, text_size, more_data, _ = jenkins.GetLogProgressive(job_name, job_build, text_size)
		if len(body) > 0 {
			fmt.Println(body)
		}
		time.Sleep(3000 * time.Millisecond)
	}
	return nil
}

func (jenkins *Jenkins) GetLogProgressive(job_name string, job_build string, start string) (string, string, string, error) {
	jenkins_url_log := jenkins.Host + "/job/" + job_name + "/" + job_build + "/logText/progressiveText?start=" + start
	req, err := http.NewRequest("GET", jenkins_url_log, nil)
	req.SetBasicAuth(jenkins.User, jenkins.Token)
	client := &http.Client{}
	resp, err := client.Do(req)
	text_size := strings.Join(resp.Header["X-Text-Size"], "")
	more_data := strings.Join(resp.Header["X-More-Data"], "")
	if err != nil {
		return "", "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", "", "", nil
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", "", "", err
	}
	return string(body), text_size, more_data, nil
}

func (jenkins *Jenkins) GetJob(url_queue string) (string, error) {
	jenkins_url_queue := url_queue + "api/json"
	req, err := http.NewRequest("POST", jenkins_url_queue, nil)
	if err != nil {
		panic(err)
	}
	req.SetBasicAuth(jenkins.User, jenkins.Token)
	req.Header.Set(jenkins.CrumbHeader, jenkins.Crumb)
	client := &http.Client{}
	timer_chan := time.NewTimer(time.Second * 60).C
	ticker := time.NewTicker(time.Second * 5)
	defer ticker.Stop()
	var build_url string
	var build_found bool
	go func() {
		for _ = range ticker.C {
			resp, err := client.Do(req)
			defer resp.Body.Close()
			fmt.Println("response Status:", resp.StatusCode)
			//fmt.Println("timer:", t)
			if err != nil {
				panic(err)
			}
			if resp.StatusCode == 200 {
				body, _ := ioutil.ReadAll(resp.Body)
				json_queue, err := gabs.ParseJSON(body)
				if err != nil {
					panic(err)
				}
				build_url, build_found = json_queue.Path("executable.url").Data().(string)
			}
		}
	}()
	for {
		select {
		case <-timer_chan:
			fmt.Println("Timer expired")
			return "", errors.New("Time expired waiting for job to be in the queue")
		case <-ticker.C:
			if build_found {
				return build_url, nil
			}
		}

	}
	return "", nil
}

func CmdBuildJob(job_name string) error {
	var jenkins Jenkins
	jenkins.Host, jenkins.User, jenkins.Token, jenkins.CrumbHeader, jenkins.Crumb, _ = CfgGetCredentials(cfg_profile)
	_, err := jenkins.BuildJob(job_name)

	if err != nil {
		panic(err)
	}
	fmt.Println("Success!")
	return nil
}

func CmdGetLog(job_name string, job_build string) error {
	var jenkins Jenkins
	jenkins.Host, jenkins.User, jenkins.Token, jenkins.CrumbHeader, jenkins.Crumb, _ = CfgGetCredentials(cfg_profile)
	err := jenkins.GetLog(job_name, job_build)

	if err != nil {
		panic(err)
	}
	return nil
}

func CmdListJobs() error {
	var jenkins Jenkins
	var status string
	jenkins.Host, jenkins.User, jenkins.Token, jenkins.CrumbHeader, jenkins.Crumb, _ = CfgGetCredentials(cfg_profile)

	jobs, err := jenkins.ListJobs()

	if err != nil {
		return err
	}
	output := []string{
		"JOB NAME |STATUS",
	}
	for i := range jobs {
		switch jobs[i].Color {
		case "red":
			status = "Failed"
		case "blue", "blue_anime":
			status = "Success"
		case "notbuilt":
			status = "Not built"
		default:
			status = strings.Title(jobs[i].Color)
		}
		job_cols := jobs[i].Name + "|" + status
		output = append(output, job_cols)
	}
	result := columnize.SimpleFormat(output)
	fmt.Println(result)
	return nil
}

func CmdConfigure() error {
	input_host := bufio.NewReader(os.Stdin)
	fmt.Print("Jenkins Host: ")
	data_host, _ := input_host.ReadString('\n')
	JenkinsHost := strings.TrimRight(data_host, "\r\n")
	input_user := bufio.NewReader(os.Stdin)
	fmt.Print("Jenkins User: ")
	data_user, _ := input_user.ReadString('\n')
	JenkinsUser := strings.TrimRight(data_user, "\r\n")
	input_token := bufio.NewReader(os.Stdin)
	fmt.Print("Jenkins Token: ")
	data_token, _ := input_token.ReadString('\n')
	JenkinsToken := strings.TrimRight(data_token, "\r\n")
	//fmt.Print("Jenkins Crumb: ")
	//data_crumb, _ := input_token.ReadString('\n')
	//jenkins_crumb := strings.TrimRight(data_crumb, "\r\n")
	JenkinsCrumb, _ := GetCrumb(JenkinsHost, JenkinsUser, JenkinsToken)
	JenkinsCrumbHV := strings.Split(JenkinsCrumb, ":")
	JenkinsCrumbHeader, JenkinsCrumbValue := JenkinsCrumbHV[0], JenkinsCrumbHV[1]
	CfgSaveCredentials(cfg_profile, JenkinsHost, JenkinsUser, JenkinsToken, JenkinsCrumbHeader, JenkinsCrumbValue)

	return nil
}

func CfgSaveCredentials(profile string, host string, user string, token string, crumbHeader string, crumb string) error {

	cfg, err := ini.Load(cfg_file)
	if err != nil {
		//If file or dir doesn't exist create them
		if strings.HasSuffix(err.Error(), "no such file or directory") {
			CfgCreateCredentialsFile(cfg_file)
			cfg, err = ini.Load(cfg_file)
			if err != nil {
				panic(err)
			}
		} else {
			panic(err)
		}
	}
	cfg.Section(profile).Key("jenkins_host").SetValue(host)
	cfg.Section(profile).Key("jenkins_user").SetValue(user)
	cfg.Section(profile).Key("jenkins_token").SetValue(token)
	cfg.Section(profile).Key("jenkins_crumb_header").SetValue(crumbHeader)
	cfg.Section(profile).Key("jenkins_crumb").SetValue(crumb)

	err = cfg.SaveTo(cfg_file)
	if err != nil {
		panic(err)
	}
	return nil
}

func CfgGetCredentials(profile string) (string, string, string, string, string, error) {
	cfg, err := ini.Load(cfg_file)
	if err != nil {
		panic(err)
	}
	host := cfg.Section(profile).Key("jenkins_host").String()
	user := cfg.Section(profile).Key("jenkins_user").String()
	token := cfg.Section(profile).Key("jenkins_token").String()
	crumb := cfg.Section(profile).Key("jenkins_crumb").String()
	crumb_header := cfg.Section(profile).Key("jenkins_crumb_header").String()
	return host, user, token, crumb_header, crumb, nil

}

func CfgCreateCredentialsFile(file string) error {
	config_dir := filepath.Dir(file)
	os.Mkdir(config_dir, 0755)
	if _, err := os.Stat(file); os.IsNotExist(err) {
		f, err := os.Create(file)
		if err != nil {
			panic(err)
		}
		f.Close()
	}
	return nil
}

func main() {
	app := cli.NewApp()
	app.Version = version
	app.Name = "jenkinson"
	app.Usage = "A CLI tool to manage Jenkins"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:        "profile",
			Value:       "default",
			Usage:       "Select a Jenkins Profile",
			Destination: &cfg_profile,
		},
	}

	app.Commands = []cli.Command{
		{
			Name:  "build",
			Usage: "build job",
			Action: func(c *cli.Context) error {
				if c.NArg() == 1 {
					CmdBuildJob(c.Args().First())
				} else {
					fmt.Println("error")
				}

				return nil
			},
		},
		{
			Name:  "configure",
			Usage: "configure",
			Action: func(c *cli.Context) error {
				CmdConfigure()
				return nil
			},
		},
		{
			Name:  "jobs",
			Usage: "list jobs",
			Action: func(c *cli.Context) error {
				CmdListJobs()
				return nil
			},
		},
		{
			Name:  "log",
			Usage: "get build log",
			Action: func(c *cli.Context) error {
				if c.NArg() == 2 {
					CmdGetLog(c.Args().First(), c.Args()[1])
				} else {
					fmt.Println("error")
				}

				return nil
			},
		},
		/*{
			Name:   "crumb",
			Usage:  "get crumb",
			Hidden: true,
			Action: func(c *cli.Context) error {
				CmdGetCrumb()
				return nil

			},
		},*/
	}

	app.Run(os.Args)
}
