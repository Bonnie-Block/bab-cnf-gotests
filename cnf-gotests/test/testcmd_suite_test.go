package testcmd_test

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/containers/podman/v2/libpod/define"
	"github.com/containers/podman/v2/pkg/api/handlers"
	"github.com/containers/podman/v2/pkg/bindings"
	"github.com/containers/podman/v2/pkg/bindings/containers"
	"github.com/containers/podman/v2/pkg/bindings/network"
	"github.com/containers/podman/v2/pkg/domain/entities"
	"github.com/containers/podman/v2/pkg/specgen"
	"github.com/docker/docker/api/types"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
)

const (
	NegativeFlag string = "-negative"

	SkipFlag string = "-skip"

	MTUFlag         string = "-mtu=1400"
	NegativeMTUFlag string = "-mtu=2000"

	TCPFlag  string = "-protocol=tcp"
	UDPFlag  string = "-protocol=udp"
	SCTPFlag string = "-protocol=sctp"
	ICMPFlag string = "-protocol=icmp"

	MulticastFlag string = "-multicast"
	BroadcastFlag string = "-broadcast"

	ServerFlag string = "-listen"

	PortFlag string = "-port=1450"

	ServerAddressFlag string = "-server="

	IPV4MulticastAddressFlag string = "-server=224.255.0.10"
	BroadcastAddressFlag     string = "-server=255.255.255.255"

	IPV6MulticastAddressFlag string = "-server=FF05:0:0:0:0:0:0:18C"

	InterfaceNameFlag string = "-interface=eth0"

	ContainerImage   string = "localhost/cnf-gotests-client:latest"
	ContainerNetwork string = "testcmdTest"
)

var (
	IPV4ServerAddressFlag string = ServerAddressFlag
	IPV6ServerAddressFlag string = ServerAddressFlag
	ServerContainerID     string = ""
	ClientContainerID     string = ""
)

const TestCmd = "testcmd"

var connText context.Context

func init() {
	var err error

	socket := "unix:" + "/run" + "/podman/podman.sock"
	connText, err = bindings.NewConnection(context.Background(), socket)
	if err != nil {
		log.Fatalf("%v", err)
	}

	networkName := ContainerNetwork

	_, netIP, err := net.ParseCIDR("3ffe:ffff:0:1235::/64")
	if err != nil {
		log.Fatalf("%v", err)
	}

	_, err = network.Create(connText,
		entities.NetworkCreateOptions{
			IPv6:   true,
			Subnet: *netIP,
		},
		&networkName,
	)
	if err != nil {
		log.Fatalf("%v", err)
	}

	serverContainer, err := createContainer(ContainerNetwork)
	if err != nil {
		log.Fatalf("%v", err)
	}
	clientContainer, err := createContainer(ContainerNetwork)
	if err != nil {
		log.Fatalf("%v", err)
	}

	err = containers.Start(connText, serverContainer.ID, nil)
	if err != nil {
		log.Fatalf("%v", err)
	}
	err = containers.Start(connText, clientContainer.ID, nil)
	if err != nil {
		log.Fatalf("%v", err)
	}

	ServerContainerID = serverContainer.ID
	ClientContainerID = clientContainer.ID

	running := define.ContainerStateRunning
	_, err = containers.Wait(connText, serverContainer.ID, &running)
	if err != nil {
		log.Fatalf("%v", err)
	}
	_, err = containers.Wait(connText, clientContainer.ID, &running)
	if err != nil {
		log.Fatalf("%v", err)
	}

	serverContainerData, err := containers.Inspect(connText, serverContainer.ID, nil)
	if err != nil {
		log.Fatalf("%v", err)
	}

	networkKeys := []string{}
	for key := range serverContainerData.NetworkSettings.Networks {
		networkKeys = append(networkKeys, key)
	}
	if len(networkKeys) == 0 {
		log.Fatalln("No network was defined for the server container")
	}

	networkSettings := serverContainerData.NetworkSettings.Networks[networkKeys[0]]

	IPV4ServerAddressFlag = fmt.Sprintf("%s%s", IPV4ServerAddressFlag, networkSettings.IPAddress)
	if IPV4ServerAddressFlag == ServerAddressFlag {
		log.Fatalln("IPV4 address error")
	}
	IPV6ServerAddressFlag = fmt.Sprintf("%s%s", IPV6ServerAddressFlag, networkSettings.GlobalIPv6Address)
	if IPV6ServerAddressFlag == ServerAddressFlag {
		log.Fatalln("IPV6 address error")
	}
}

func TestTestCmd(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "testcmd Test Suite")
}

var _ = AfterSuite(func() {
	var timeout uint = 10

	err := containers.Stop(connText, ServerContainerID, &timeout)
	Expect(err).NotTo(HaveOccurred())
	err = containers.Stop(connText, ClientContainerID, &timeout)
	Expect(err).NotTo(HaveOccurred())

	_, err = containers.Wait(connText, ServerContainerID, nil)
	Expect(err).NotTo(HaveOccurred())
	_, err = containers.Wait(connText, ClientContainerID, nil)
	Expect(err).NotTo(HaveOccurred())

	removeWithForce := true

	_, err = network.Remove(connText, ContainerNetwork, &removeWithForce)
	Expect(err).NotTo(HaveOccurred())
})

var _ = Describe("testcmd", func() {
	IPV4Protocols := [][]string{
		{SCTPFlag},
		{UDPFlag},
		{UDPFlag, MulticastFlag},
		{UDPFlag, BroadcastFlag},
		{TCPFlag},
		{ICMPFlag},
	}

	DescribeTable("IPV4",
		runTestExec,
		createIPV4Entries(IPV4Protocols)...,
	)

	IPV6Protocols := [][]string{
		{SCTPFlag},
		{UDPFlag},
		{UDPFlag, MulticastFlag},
		{TCPFlag},
		{ICMPFlag},
	}

	DescribeTable("IPV6",
		runTestExec,
		createIPV6Entries(IPV6Protocols)...,
	)
})

func createIPV4Entries(protocols [][]string) (entries []TableEntry) {
	for _, protocol := range protocols {
		flags := make([]string, 0)
		flags = append(flags, protocol...)
		flags = append(flags, InterfaceNameFlag)

		serverAddressFlag := IPV4ServerAddressFlag
		portFlag := PortFlag

		positiveFlags := flags
		negativeFlags := flags

		switch protocol[0] {
		case UDPFlag:
			{
				if len(protocol) > 1 {
					switch protocol[1] {
					case MulticastFlag:
						{
							serverAddressFlag = IPV4MulticastAddressFlag
						}
					case BroadcastFlag:
						{
							serverAddressFlag = BroadcastAddressFlag
						}
					}
				}
			}
		case TCPFlag:
			{
				portFlag = "-port=80"
			}
		}

		positiveFlags = append(positiveFlags, serverAddressFlag, portFlag, MTUFlag)
		negativeFlags = append(negativeFlags, serverAddressFlag, portFlag, NegativeMTUFlag, NegativeFlag)

		entries = append(entries, createEntry(positiveFlags))
		entries = append(entries, createEntry(negativeFlags))
	}
	return entries
}

func createIPV6Entries(protocols [][]string) (entries []TableEntry) {
	for _, protocol := range protocols {
		flags := make([]string, 0)
		flags = append(flags, protocol...)
		flags = append(flags, InterfaceNameFlag)

		serverAddressFlag := IPV6ServerAddressFlag
		portFlag := PortFlag

		positiveFlags := flags
		negativeFlags := flags

		switch protocol[0] {
		case UDPFlag:
			{
				if len(protocol) > 1 {
					serverAddressFlag = IPV6MulticastAddressFlag
				}
			}
		case TCPFlag:
			{
				portFlag = "-port=80"
				negativeFlags = append(negativeFlags, SkipFlag)
			}
		}

		positiveFlags = append(positiveFlags, serverAddressFlag, portFlag, MTUFlag)
		negativeFlags = append(negativeFlags, serverAddressFlag, portFlag, NegativeMTUFlag, NegativeFlag)

		entries = append(entries, createEntry(positiveFlags))
		entries = append(entries, createEntry(negativeFlags))
	}
	return entries
}

func createEntry(flags []string) TableEntry {
	return Entry(createTestName(flags), flags)
}

type writer struct {
	io.Writer
}

func (w writer) Close() error {
	return nil
}

func runTestExec(flags []string) {
	if shouldSkip(flags) {
		Skip("work in progress")
	}

	if usesServer(flags) {
		By("Creating server exec")
		serverExec, err := createServerExecWithFlags(flags)
		Expect(err).NotTo(HaveOccurred())

		By("Starting server exec")
		err = containers.ExecStart(connText, serverExec)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() bool {
			data, err := containers.ExecInspect(connText, serverExec)
			Expect(err).NotTo(HaveOccurred())
			return data.Running
		}, time.Minute, time.Second).Should(Equal(true))

		defer func(serverExec string) {
			By("Cleaning server exec")
			serverCleaner, err := containers.ExecCreate(connText, ServerContainerID, &handlers.ExecCreateConfig{
				ExecConfig: types.ExecConfig{
					Cmd: []string{"pkill", "-SIGINT", TestCmd},
				},
			})
			Expect(err).NotTo(HaveOccurred())

			err = containers.ExecStart(connText, serverCleaner)
			Expect(err).NotTo(HaveOccurred())
		}(serverExec)
	}

	By("Creating client exec")
	clientExec, err := createClientExecWithFlags(flags)
	Expect(err).NotTo(HaveOccurred())

	By("Starting client exec with attach")
	streams := &define.AttachStreams{
		OutputStream: writer{GinkgoWriter},
		ErrorStream:  writer{GinkgoWriter},
		AttachOutput: true,
		AttachError:  true,
	}

	fmt.Fprintln(GinkgoWriter, "\nclient output: ")
	err = containers.ExecStartAndAttach(connText, clientExec, streams)
	fmt.Fprint(GinkgoWriter, "\n\n")
	Expect(err).NotTo(HaveOccurred())

	By("Waiting for client to finish")
	Eventually(func() bool {
		data, err := containers.ExecInspect(connText, clientExec)
		Expect(err).NotTo(HaveOccurred())
		return !data.Running
	}, time.Minute, time.Second).Should(Equal(true))

	data, err := containers.ExecInspect(connText, clientExec)
	Expect(err).NotTo(HaveOccurred())
	Expect(data.ExitCode).To(Equal(0))
}

func createServerExecWithFlags(flags []string) (string, error) {
	cmd := []string{TestCmd, ServerFlag}

	cmd = append(cmd, flags...)

	fmt.Fprintln(GinkgoWriter, "server: ", cmd)

	return containers.ExecCreate(connText, ServerContainerID, &handlers.ExecCreateConfig{
		ExecConfig: types.ExecConfig{
			Cmd: cmd,
		},
	})
}

func createClientExecWithFlags(flags []string) (string, error) {
	cmd := []string{TestCmd}

	cmd = append(cmd, flags...)

	fmt.Fprintln(GinkgoWriter, "client: ", cmd)

	return containers.ExecCreate(connText, ClientContainerID, &handlers.ExecCreateConfig{
		ExecConfig: types.ExecConfig{
			AttachStdin:  false,
			AttachStderr: true,
			AttachStdout: true,
			Cmd:          cmd,
		},
	})
}

func usesServer(args []string) bool {
	return (containsFlag(args, SCTPFlag) || containsFlag(args, UDPFlag))
}

func shouldSkip(args []string) bool {
	return containsFlag(args, SkipFlag)
}

func containsFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}

func createContainer(networkName string) (entities.ContainerCreateResponse, error) {
	spec := specgen.NewSpecGenerator(ContainerImage, false)

	spec.Terminal = true

	spec.ContainerBasicConfig = specgen.ContainerBasicConfig{
		Command: []string{"httpd", "-X"},
	}

	spec.ContainerNetworkConfig = specgen.ContainerNetworkConfig{
		NetNS: specgen.Namespace{
			NSMode: specgen.Bridge,
		},
		CNINetworks: []string{networkName},
	}

	spec.ContainerSecurityConfig = specgen.ContainerSecurityConfig{
		Privileged: true,
	}

	return containers.CreateWithSpec(connText, spec)
}

func createTestName(flags []string) string {
	cleanFlags := make([]string, len(flags))

	for i, flag := range flags {
		cleanFlags[i] = strings.TrimPrefix(flag, "-")
	}

	return strings.Join(cleanFlags, ", ")
}
