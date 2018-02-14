package dogstatsd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type dockerClient struct {
	host string
	http http.Client
}

func (c *dockerClient) listContainers() (containers []dockerContainer, err error) {
	err = c.get("/containers/json", &containers)
	return
}

func (c *dockerClient) get(url string, ret interface{}) (err error) {
	var host = dockerHostAddr(c.host)
	var req *http.Request
	var res *http.Response

	if len(host) == 0 {
		return
	}

	if req, err = http.NewRequest(http.MethodGet, host+url, nil); err != nil {
		return
	}

	if res, err = c.http.Do(req); err != nil {
		return
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		err = fmt.Errorf("%s: %s", req.URL, res.Status)
		return
	}

	err = json.NewDecoder(res.Body).Decode(ret)
	return
}

type dockerContainer struct {
	Image           dockerImage
	NetworkSettings dockerNetworkSettings
}

type dockerNetworkSettings struct {
	Networks map[string]dockerNetwork
}

type dockerNetwork struct {
	IPAddress string
}

type dockerImage string

func (image dockerImage) repo() string {
	repo, _, _ := image.parts()
	return repo
}

func (image dockerImage) name() string {
	_, name, _ := image.parts()
	return name
}

func (image dockerImage) version() string {
	_, _, version := image.parts()
	return version
}

func (image dockerImage) parts() (repo, name, version string) {
	s := string(image)
	i := strings.LastIndexByte(s, ':')
	if i < 0 {
		name = s
	} else {
		name, version = s[:i], s[i+1:]
	}
	j := strings.LastIndexByte(name, '/')
	if j >= 0 {
		repo, name = name[:j], name[j+1:]
	}
	return
}

func dockerHostAddr(host string) string {
	if len(host) != 0 && !strings.HasPrefix(host, "http://") {
		i := strings.Index(host, "://")
		if i >= 0 {
			host = host[i+3:]
		}
		host = "http://" + host
	}
	return host
}
