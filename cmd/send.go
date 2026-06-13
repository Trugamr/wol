package cmd

import (
	"fmt"
	"log"
	"net"

	"github.com/spf13/cobra"
	"github.com/trugamr/wol/magicpacket"
)

func init() {
	rootCmd.AddCommand(sendCmd)

	sendCmd.Flags().StringP("mac", "m", "", "MAC address of the device to wake up")
	sendCmd.Flags().StringP("name", "n", "", "Name of the device to wake up")
	sendCmd.Flags().StringP("broadcast", "b", "", "Broadcast address to send the magic packet to (overrides config)")
	sendCmd.Flags().IntP("port", "p", 0, "UDP port to send the magic packet to (overrides config)")
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
		// Default to the global broadcast target; the --name path may refine it
		// with the machine's per-machine override.
		broadcast := cfg.Broadcast

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
			machine, err := cfg.MachineByName(name)
			if err != nil {
				cobra.CheckErr(err)
			}

			mac, err = machine.HardwareAddr()
			if err != nil {
				cobra.CheckErr(err)
			}
			broadcast = cfg.BroadcastFor(machine)
		default:
			log.Fatalf("mac address should come from either --mac or --name")
		}

		// Command-line flags take precedence over config.
		if cmd.Flags().Changed("broadcast") {
			value, err := cmd.Flags().GetString("broadcast")
			if err != nil {
				cobra.CheckErr(err)
			}
			broadcast.Address = value
		}
		if cmd.Flags().Changed("port") {
			value, err := cmd.Flags().GetInt("port")
			if err != nil {
				cobra.CheckErr(err)
			}
			broadcast.Port = value
		}

		addr := broadcast.Addr()
		log.Printf("Sending magic packet to %s via %s", mac, addr)
		mp := magicpacket.NewMagicPacket(mac)
		if err := mp.Broadcast(addr); err != nil {
			cobra.CheckErr(err)
		}

		log.Printf("Magic packet sent")
	},
}
