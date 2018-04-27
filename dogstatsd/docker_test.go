package dogstatsd

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestDockerImage(t *testing.T) {
	tests := []struct {
		image   dockerImage
		repo    string
		name    string
		version string
	}{
		{
			image:   "",
			repo:    "",
			name:    "",
			version: "",
		},

		{
			image:   "coredns",
			repo:    "",
			name:    "coredns",
			version: "",
		},

		{
			image:   "segment/coredns",
			repo:    "segment",
			name:    "coredns",
			version: "",
		},

		{
			image:   "coredns:1.0.5",
			repo:    "",
			name:    "coredns",
			version: "1.0.5",
		},

		{
			image:   "segment/coredns:1.0.5",
			repo:    "segment",
			name:    "coredns",
			version: "1.0.5",
		},
	}

	for _, test := range tests {
		t.Run(string(test.image), func(t *testing.T) {
			repo, name, version := test.image.parts()

			if repo != test.repo {
				t.Error("repo mismatch:", repo, "!=", test.repo)
			}

			if name != test.name {
				t.Error("name mismatch:", name, "!=", test.name)
			}

			if version != test.version {
				t.Error("version mismatch:", version, "!=", test.version)
			}
		})
	}
}

func TestDockerNetworkAddress(t *testing.T) {
	tests := []struct {
		host    string
		network string
		address string
	}{
		{
			host:    "",
			network: "",
			address: "",
		},

		{
			host:    "/var/run/docker.sock",
			network: "unix",
			address: "/var/run/docker.sock",
		},

		{
			host:    "unix:///var/run/docker.sock",
			network: "unix",
			address: "/var/run/docker.sock",
		},

		{
			host:    "localhost",
			network: "tcp",
			address: "localhost:2376",
		},

		{
			host:    "tcp://localhost",
			network: "tcp",
			address: "localhost:2376",
		},

		{
			host:    "tcp://localhost:2376",
			network: "tcp",
			address: "localhost:2376",
		},
	}

	for _, test := range tests {
		network, address := dockerNetworkAddress(test.host)

		if network != test.network {
			t.Errorf("%s: invalid network: %s != %s", test.host, network, test.network)
		}

		if address != test.address {
			t.Errorf("%s: invalid address: %s != %s", test.host, network, test.address)
		}
	}
}

func TestDockerClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/containers/json" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		w.Write([]byte(`[
  {
    "Id": "89763c167db7c57a248a152877056cdefd4fb6dc1ab14113db3c42afc12e8574",
    "Names": [
      "/coredns_coredns_1"
    ],
    "Image": "segment/coredns:1.4.4",
    "ImageID": "sha256:dc671f751e39f06c516452995213445ed78e97cb3f475de42df14409bf56acc5",
    "Command": "/entrypoint.sh",
    "Created": 1518566675,
    "Ports": [
      {
        "IP": "0.0.0.0",
        "PrivatePort": 6053,
        "PublicPort": 6053,
        "Type": "tcp"
      },
      {
        "PrivatePort": 9053,
        "Type": "tcp"
      },
      {
        "IP": "0.0.0.0",
        "PrivatePort": 53,
        "PublicPort": 1053,
        "Type": "tcp"
      },
      {
        "IP": "0.0.0.0",
        "PrivatePort": 53,
        "PublicPort": 1053,
        "Type": "udp"
      }
    ],
    "Labels": {
      "com.docker.compose.config-hash": "3a875203c1620a6d8f1566605beddc87bb2b9581e2f9e364d0095be99c6ead85",
      "com.docker.compose.container-number": "1",
      "com.docker.compose.oneoff": "False",
      "com.docker.compose.project": "coredns",
      "com.docker.compose.service": "coredns",
      "com.docker.compose.version": "1.19.0-rc2"
    },
    "State": "running",
    "Status": "Up 6 hours",
    "HostConfig": {
      "NetworkMode": "coredns_vpc"
    },
    "NetworkSettings": {
      "Networks": {
        "coredns_vpc": {
          "IPAMConfig": {
            "IPv4Address": "10.5.0.4"
          },
          "Links": null,
          "Aliases": null,
          "NetworkID": "937ab3d86abb7abdf9bc7e74cf3eb2c5672257836af0dc77090b236f78d37309",
          "EndpointID": "744c0dd472509e55c4262153777a122880195edada4cc8abb0781ef2194f2758",
          "Gateway": "10.5.0.1",
          "IPAddress": "10.5.0.4",
          "IPPrefixLen": 16,
          "IPv6Gateway": "",
          "GlobalIPv6Address": "",
          "GlobalIPv6PrefixLen": 0,
          "MacAddress": "02:42:0a:05:00:04",
          "DriverOpts": null
        }
      }
    },
    "Mounts": []
  },
  {
    "Id": "5cd9f15a16621239c49549eb6c741f49c0b29dc6f8cf80a964c9e7c0bbb942c7",
    "Names": [
      "/coredns_dogstatsd_1"
    ],
    "Image": "segment/dogstatsd",
    "ImageID": "sha256:4aacb258b45b4e34717c7c1e962bae6a0ad430c3659a1290aa0387321b19372f",
    "Command": "/dogstatsd agent",
    "Created": 1518566674,
    "Ports": [
      {
        "IP": "0.0.0.0",
        "PrivatePort": 8125,
        "PublicPort": 8125,
        "Type": "udp"
      }
    ],
    "Labels": {
      "com.docker.compose.config-hash": "6ec2e9f388383ffbb18e8376f44adde36a17014830d3633a13c99d92566058f7",
      "com.docker.compose.container-number": "1",
      "com.docker.compose.oneoff": "False",
      "com.docker.compose.project": "coredns",
      "com.docker.compose.service": "dogstatsd",
      "com.docker.compose.version": "1.19.0-rc2"
    },
    "State": "running",
    "Status": "Up 6 hours",
    "HostConfig": {
      "NetworkMode": "coredns_vpc"
    },
    "NetworkSettings": {
      "Networks": {
        "coredns_vpc": {
          "IPAMConfig": {
            "IPv4Address": "10.5.0.3"
          },
          "Links": null,
          "Aliases": null,
          "NetworkID": "937ab3d86abb7abdf9bc7e74cf3eb2c5672257836af0dc77090b236f78d37309",
          "EndpointID": "f24122dcb242b5deaefb3f7d88ec65380723763488ce1e3ff5777b4f63cc869d",
          "Gateway": "10.5.0.1",
          "IPAddress": "10.5.0.3",
          "IPPrefixLen": 16,
          "IPv6Gateway": "",
          "GlobalIPv6Address": "",
          "GlobalIPv6PrefixLen": 0,
          "MacAddress": "02:42:0a:05:00:03",
          "DriverOpts": null
        }
      }
    },
    "Mounts": []
  }
]
`))
	}))
	defer server.Close()

	client := dockerClient{
		host: server.URL[7:],
	}

	containers, err := client.listContainers()
	if err != nil {
		t.Error(err)
	}

	if !reflect.DeepEqual(containers, []dockerContainer{
		{
			Image: "segment/coredns:1.4.4",
			NetworkSettings: dockerNetworkSettings{
				Networks: map[string]dockerNetwork{
					"coredns_vpc": {
						IPAMConfig: dockerIPAMConfig{
							IPv4Address: "10.5.0.4",
						},
						IPAddress: "10.5.0.4",
					},
				},
			},
		},

		{
			Image: "segment/dogstatsd",
			NetworkSettings: dockerNetworkSettings{
				Networks: map[string]dockerNetwork{
					"coredns_vpc": {
						IPAMConfig: dockerIPAMConfig{
							IPv4Address: "10.5.0.3",
						},
						IPAddress: "10.5.0.3",
					},
				},
			},
		},
	}) {
		t.Error(containers)
	}
}
