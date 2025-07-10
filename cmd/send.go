package cmd

import (
	"fmt"
	"log"
	"net"
	"strings"

	"github.com/spf13/cobra"
	"github.com/trugamr/wol/config"
	"github.com/trugamr/wol/magicpacket"
)

func init() {
	rootCmd.AddCommand(sendCmd)

	sendCmd.Flags().StringP("mac", "m", "", "MAC address of the device to wake up")
	sendCmd.Flags().StringP("name", "n", "", "Name of the device to wake up")
}

var sendCmd = &cobra.Command{
	Use:   "send",
	Short: "Send a magic packet to specified mac address",
	Long:  "Send a magic packet to wake up a device on the network using the specified mac address",
	Args:  cobra.NoArgs,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		// Only one of the flags should be specified
		if cmd.Flags().Changed("mac") == cmd.Flags().Changed("name") {
			return fmt.Errorf("either --mac or --name must be specified")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		var mac net.HardwareAddr

		// Retrieve mac address using one of the flags
		switch {
		case cmd.Flags().Changed("mac"):
			value, err := cmd.Flags().GetString("mac")
			if err != nil {
				cobra.CheckErr(err)
			}

			mac, err = net.ParseMAC(value)
			if err != nil {
				cobra.CheckErr(err)
			}
		case cmd.Flags().Changed("name"):
			// Get the name of the machine
			name, err := cmd.Flags().GetString("name")
			if err != nil {
				cobra.CheckErr(err)
			}

			// Find machine with the specified name
			machine, err := getMachineByName(name)
			if err != nil {
				cobra.CheckErr(err)
			}
			mac, err = getMac(machine)
			if err != nil {
				cobra.CheckErr(err)
			}
		default:
			log.Fatalf("mac address should come from either --mac or --name")
		}

		log.Printf("Sending magic packet to %s", mac)
		mp := magicpacket.NewMagicPacket(mac)
		if err := mp.Broadcast(); err != nil {
			cobra.CheckErr(err)
		}

		log.Printf("Magic packet sent")
	},
}

// getMachineByName returns the machine with the specified name
func getMachineByName(name string) (*config.Machine, error) {
	for _, machine := range cfg.Machines {
		if strings.EqualFold(machine.Name, name) {
			return &machine, nil
		}
	}

	return nil, fmt.Errorf("machine with name %q not found", name)
}

// getMac returns the MAC address of the specified machine
func getMac(machine *config.Machine) (net.HardwareAddr, error) {
	mac, err := net.ParseMAC(machine.Mac)
	if err != nil {
		return nil, fmt.Errorf("failed to parse MAC address: %w", err)
	}
	return mac, nil
}
