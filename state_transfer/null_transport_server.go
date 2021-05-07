package state_transfer

import (
	"strconv"

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (s *NullTransport) createTransportServerResources(c client.Client, t Transfer) error {
	s.SetPort(socatPort)

	createNullServerContainers(s, t)

	return nil
}

func createNullServerContainers(s *NullTransport, t Transfer) {
	s.SetServerContainers([]v1.Container{
		{
			Name:  "null",
			Image: socatImage,
			Command: []string{
				"/usr/bin/socat",
				"TCP4-LISTEN:" +
					strconv.Itoa(int(socatPort)) +
					",fork,reuseaddr",
				"TCP4:localhost:" +
					strconv.Itoa(int(t.Port())),
			},
			Ports: []v1.ContainerPort{
				{
					Name:          "null",
					Protocol:      v1.ProtocolTCP,
					ContainerPort: socatPort,
				},
			},
		},
	})
}
