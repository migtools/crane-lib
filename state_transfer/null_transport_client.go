package state_transfer

import (
	"strconv"

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (s *NullTransport) createTransportClientResources(c client.Client, t Transfer) error {
	createNullClientContainers(s, t)

	return nil
}

func createNullClientContainers(s *NullTransport, t Transfer) {
	s.SetClientContainers([]v1.Container{
		{
			Name:  "null",
			Image: socatImage,
			Command: []string{
				"/usr/bin/socat",
				"TCP4-LISTEN:" +
					strconv.Itoa(int(socatPort)) +
					",fork,reuseaddr",
				"TCP4:" +
					t.GetEndpoint().GetHostname() + ":" +
					strconv.Itoa(int(socatPort)),
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
