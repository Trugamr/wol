package cmd

import (
	"fmt"
	"log"
	"net"

	"github.com/spf13/cobra"
	"github.com/trugamr/wol/wake"
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
		switch true {
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
			mac, err = wake.ByName(cfg.Machines, name)
			if err != nil {
				cobra.CheckErr(err)
			}
		default:
			log.Fatalf("mac address should come from either --mac or --name")
		}

		log.Printf("Sending magic packet to %s", mac)
		if err := wake.Broadcast(mac); err != nil {
			cobra.CheckErr(err)
		}

		log.Printf("Magic packet sent")
	},
}
