package main

import (
    "flag"
    "fmt"
    "os"
    "os/exec"
    "time"
    "strings"
)

var backend, vniId, devName string
var qlen, dstport string

func runcmd(path string, args []string, debug bool) (output string, err error) {
    cmd := exec.Command(path, args...)

    var b []byte
    b, err = cmd.CombinedOutput()
    out := string(b)

    if debug {
        fmt.Println(strings.Join(cmd.Args[:], " "))
        if err != nil {
            fmt.Println("runcmd error")
            fmt.Println(out)
        }
    }
    return out, err
}

func splitArray(in []string, num int) [][]string {
    var divided [][]string

    chunk := (len(in) + num - 1) / num
    for i := 0; i < len(in); i += chunk {
        end := i + chunk
        if end > len(in) {
            end = len(in)
        }
        divided = append(divided, in[i:end])
    }
    return divided
}

func check(e error) {
    if e != nil {
        panic(e)
    }
}

func tunnel_cni_conf(network string, subnet string, file string) int {
    f, err := os.Create(file) 
    check(err)

    str := fmt.Sprintf("{\n    \"cniVersion\": \"0.3.1\",\n    \"name\": \"mynet\",\n    \"type\": \"tunnel-cni\",\n    \"network\": \"%s\",\n    \"subnet\": \"%s\"\n}\n", network, subnet)
    num, err := f.WriteString(str)
    check(err)
    f.Sync()
    f.Close()
    return num
}

func vtepmac(addr, ifname, name, ifaddr string) {
    var output, tmpstr, macaddr, cmdStr string
    var err error
    var args []string

    // Create device
    if backend == "vxlan" {
        devName = "vxlan0"
        args = []string{"link", "show", devName}
        output, err = runcmd("ip", args, true)
        if err == nil {
            cmdStr = fmt.Sprintf("ip -4 -o addr show %s | awk '/inet/ {print $4}'", devName)
            args = []string{"-c", cmdStr}
            output, err = runcmd("sh", args, true)
            tmpstr = strings.TrimSuffix(output, "\n")
            if err != nil {
                fmt.Println("Error:", cmdStr, output)
            }
            fmt.Println("ifaddr:", ifaddr, ", tmpstr:", tmpstr)
            if ifaddr != tmpstr {
                args = []string{"addr", "delete", tmpstr, "dev", devName}
                output, err = runcmd("ip", args, true)
                if err != nil {
                    fmt.Println("Error: delete", devName, "address", output)
                }
                args = []string{"addr", "add", ifaddr, "dev", devName}
                output, err = runcmd("ip", args, true)
                if err != nil {
                    fmt.Println("Error: add", devName, "address", output)
                }
            }
        } else {
            // Add vxlan device
            cmdStr = fmt.Sprintf("ip link add %s type vxlan id %s local %s dev %s dstport %s nolearning", devName, vniId, addr, ifname, dstport)
            args = []string{"-c", cmdStr}
            output, err = runcmd("sh", args, true)
            if err != nil {
                fmt.Println("Error:", cmdStr, output)
            }
        }
    } else if backend == "geneve" {
        devName = "geneve0"
        args = []string{"link", "add", devName, "type", backend, "id", vniId, "remote", addr, "dstport", dstport}
        output, err = runcmd("ip", args, true)
        if err != nil {
            fmt.Println("Error:", output)
            check(err)
        }
    }

    if qlen != "1000" {
        args = []string{"link", "set", devName, "txqueuelen", qlen}
        output, err = runcmd("ip", args, true)
        if err != nil {
            fmt.Println("Error:", output)
            check(err)
        }
    }

    // Obtain the MAC address of the device
    cmdStr = fmt.Sprintf("ip -4 -o link show %s | cut -d ' ' -f 19", devName)
    args = []string{"-c", cmdStr}
    output, err = runcmd("sh", args, true)
    if err != nil {
        fmt.Println("Error: ip", cmdStr, output)
        check(err)
    }
    macaddr = strings.TrimSuffix(output, "\n")
    fmt.Println("MAC address of device", devName, "is", macaddr)

    // Annotate and submit the MAC address
    cmdStr = fmt.Sprintf("kubectl annotate nodes %s vtepMAC='%s' --overwrite", name, macaddr)
    args = []string{"-c", cmdStr}
    output, err = runcmd("sh", args, true)
    if err != nil {
        fmt.Println("Error:", output)
        check(err)
    }
}

func main() {
    var gw_cidr, output, ip_address, fileName, network, cmdStr, ifname, tmpstr, gw_address string
    var err error
    var numBytes int
    var args, out, out1, macaddr_array, subnet []string
    var address, ifip [][]string
    var match bool = false
    var done int = 0
    var donevtep int = 0
    var index int = -1
    var num_nodes, number_nodes int
    
    // Setup default values
    flag.StringVar(&backend, "backend", "vxlan", "backend")
    flag.StringVar(&vniId, "id", "1", "VNI")
    flag.StringVar(&dstport, "dstport", "8472", "dstport")
    flag.StringVar(&qlen, "qlen", "0", "txqueuelen")
    flag.Parse()
    fmt.Println("Tunneling:", backend, ", id:", vniId, ", dstport:", dstport, ", qlen:", qlen)

loop:
    args = []string{"-c", "kubectl get nodes -o json | jq -r '.items[].spec.podCIDR'"}
    output, err = runcmd("sh", args, true)
    if err != nil {
        fmt.Println("Error:", output)
    }
    // subnet array 
    subnet = strings.Split(strings.TrimSuffix(output, "\n"), "\n")
    number_nodes = num_nodes
    num_nodes = len(subnet)
    fmt.Println("subnet[]:", subnet)

    args = []string{"-c", "kubectl get nodes -o json | jq -r '.items[].status.addresses[].address'"}
    output, err = runcmd("sh", args, true)
    if err != nil {
        fmt.Println("Error:", output)
    }
    // address[][] array
    out = strings.Split(strings.TrimSuffix(output, "\n"), "\n")
    address = splitArray(out, len(subnet))
    fmt.Println("address[][]", address)

    if done == 0 {
        number_nodes = num_nodes
        // This should always succeed.
        args = []string{"-c", "kubectl get pods -n kube-system -o json | grep cluster-cidr | awk 'BEGIN {FS=\"[ =\\\"]+\"} {print $3}'"}
        output, err = runcmd("sh", args, true)
        if err != nil {
            fmt.Println("Error:", output)
        }
        // Network CIDR
        network = strings.TrimSuffix(output, "\n")
        fmt.Println("network-cidr:", network)
        // Find VM name 
        output, err = os.Hostname()
        if err != nil {
            fmt.Println("Error: os.Hostname()")
        }
        fmt.Println("VM name:", output)
        for i := 0; i < len(address); i++ {
            if output == address[i][1] {
                index = i
                break
            }
        }
        fmt.Println("Index:", index)

        // Write to /etc/cni/net.d/10-tunnel-cni-plugin.conf
        fileName = "/etc/cni/net.d/10-tunnel-cni-plugin.conf"
        numBytes = tunnel_cni_conf(network, subnet[index], fileName)
        fmt.Println(numBytes, "bytes written to", fileName)

        // Find the interface name for IP address address[index][0]
        args = []string{"-c", "ip -4 -o a | cut -d ' ' -f 2,7 | cut -d '/' -f 1"}
        output, err = runcmd("sh", args, true)
        if err != nil {
            fmt.Println("Error:", output)
        }
        out = strings.Split(strings.TrimSuffix(output, "\n"), "\n")
        for i := 0; i < len(out); i++ {
            if out[i] != "" {
                rowifip := strings.Split(out[i], " ")
                ifip = append(ifip, rowifip)
            }
        }
        fmt.Println("ifip array:", ifip)

        // Find the ifname
        for i := 0; i < len(ifip); i++ {
            if address[index][0] == ifip[i][1] {
                ifname = ifip[i][0]
                break
            }
        }
        fmt.Println("ifname:", ifname)

        out = strings.Split(subnet[index], "/")
        ip_address := out[0]
        mask_size := out[1]
        fmt.Println("mask_size:",mask_size)

        slen := len(ip_address)
        if slen > 0 && ip_address[slen - 1] == '0' {
            gw_address = ip_address[:slen - 1] + "1"
        }
        gw_cidr = gw_address + "/" + mask_size
        fmt.Println(ip_address, gw_address, gw_cidr)

        if backend == "vxlan" {
            cmdStr = fmt.Sprintf("%s/32", ip_address)
            vtepmac(address[index][0], ifname, address[index][1], cmdStr)
        }

        // Bring up devName
        args = []string{"link", "set", devName, "up"}
        output, err = runcmd("ip", args, true)
        if err != nil {
            fmt.Println("Error:", output)
        }

        args = []string{"link", "show", "cni0"}
        _, err = runcmd("ip", args, true)
        if err == nil {
            args = []string{"-c", "ip addr show cni0 | awk '/inet / {print $2}'"}
            output, err = runcmd("sh", args, true)
            tmpstr = strings.TrimSuffix(output, "\n")
            if err != nil {
                fmt.Println("Error: ip addr show cni0 | awk '/inet / {print $2}'" )
            }
            fmt.Println(gw_cidr, tmpstr)
            if gw_cidr != tmpstr {
                args = []string{"addr", "delete", tmpstr, "dev", "cni0"}
                _, err = runcmd("ip", args, true)
                if err != nil {
                    fmt.Println("Error: delete cni0 address" )
                }
                args = []string{"addr", "add", gw_cidr, "dev", "cni0"}
                _, err = runcmd("ip", args, true)
                if err != nil {
                    fmt.Println("Error: add cni0 address" )
                }
            }
        } else {
            // Add bridge cni0
            args = []string{"link", "add", "cni0", "type", "bridge"}
            _, err = runcmd("ip", args, true)
            if err != nil {
                fmt.Println("Error: ip link add cni0 type bridge")
            }
            args = []string{"link", "set", "cni0", "up"}
            _, err = runcmd("ip", args, true)
            if err != nil {
                fmt.Println("Error: ip link set cni0 up")
            }
            args = []string{"addr", "add", gw_cidr, "dev", "cni0"}
            _, err = runcmd("ip", args, true)
            if err != nil {
                fmt.Println("Error: ip addr add", gw_cidr, "dev cni0")
            }
        }

        // Add iptables entries
        cmdStr = fmt.Sprintf("iptables -t filter -A FORWARD -s %s -j ACCEPT", network)
        args = []string{"-c", cmdStr}
        _, err = runcmd("sh", args, true)
        if err != nil {
            fmt.Println("Error:", cmdStr)
        }
        cmdStr = fmt.Sprintf("iptables -t filter -A FORWARD -d %s -j ACCEPT", network)
        args = []string{"-c", cmdStr}
        _, err = runcmd("sh", args, true)
        if err != nil {
            fmt.Println("Error:", cmdStr) 
        }
        cmdStr = fmt.Sprintf("-t nat -A POSTROUTING -s %s -d %s -j RETURN", network, network)
        args = []string{"-t", "nat", "-A", "POSTROUTING", "-s", network, "-d", network, "-j", "RETURN"}
        _, err = runcmd("iptables", args, true)
        if err != nil {
            fmt.Println("Error: iptables", cmdStr)
        }
        cmdStr = fmt.Sprintf("-t nat -A POSTROUTING -s %s ! -d 224.0.0.0/4 -j MASQUERADE --random-fully", network)
        args = []string{"-t", "nat", "-A", "POSTROUTING", "-s", network, "!", "-d", "224.0.0.0/4", "-j", "MASQUERADE", "--random-fully"}
        _, err = runcmd("iptables", args, true)
        if err != nil {
            fmt.Println("Error: iptables", cmdStr)
        }
        cmdStr = fmt.Sprintf("-t nat -A POSTROUTING ! -s %s -d %s -j RETURN", network, subnet[index])
        args = []string{"-t", "nat", "-A", "POSTROUTING", "!", "-s", network, "-d", subnet[index], "-j", "RETURN"}
        _, err = runcmd("iptables", args, true)
        if err != nil {
            fmt.Println("Error: iptables", cmdStr)
        }
        cmdStr = fmt.Sprintf("-t nat -A POSTROUTING ! -s %s -d %s -j MASQUERADE --random-fully", network, network)
        args = []string{"-t", "nat", "-A", "POSTROUTING", "!", "-s", network, "-d", network, "-j", "MASQUERADE", "--random-fully"}
        _, err = runcmd("iptables", args, true)
        if err != nil {
            fmt.Println("Error: iptables", cmdStr)
        }

        // Setup ip route to other VMs
        args = []string{"route"}
        output, err = runcmd("ip", args, true)
        if err != nil {
            fmt.Println("Error:", cmdStr)
        }
        out = strings.Split(strings.TrimSuffix(output, "\n"), "\n")
        for i := 0; i < len(subnet); i++ {
            if i != index && subnet[i] != "" {
                out1 = strings.Split(subnet[i], "/")
                ip_address = out1[0]
                cmdStr = fmt.Sprintf("%s via %s dev %s onlink", subnet[i], ip_address, devName)
                match = false
                for j := 0; j < len(out); j++ {
                    if strings.Contains(out[j], cmdStr) {
                        match = true
                        break
                    }
                }
                if match == false {
                    args = []string{"route", "add", subnet[i], "via", ip_address, "dev", devName, "onlink"}
                    _, err = runcmd("ip", args, true)
                    if err != nil {
                        fmt.Println("Error: ip route add", subnet[i], "via", ip_address, "dev", devName, "onlink")
                    }
                }
            }
        }

        // FDB and arp entries for vxlan backend
        if backend == "vxlan" {
loop_vxlan:
            // Get vtepMAC addresses
            args = []string{"-c", "kubectl get nodes -o json | jq -r '.items[].metadata.annotations.vtepMAC'"}
            if donevtep == 0 {
                output, err = runcmd("sh", args, true)
                donevtep = 1
            } else {
                output, err = runcmd("sh", args, false)
            }
            if err != nil {
                fmt.Println("Error:", output)
            }
            // macaddr array
            macaddr_array = strings.Split(strings.TrimSuffix(output, "\n"), "\n")

            for i := 0; i < len(macaddr_array); i++ {
                if macaddr_array[i] == "null" || macaddr_array[i] == "" {
                    time.Sleep(time.Second)
                    goto loop_vxlan
                }
            }

            // Create bridge fdb and arp entries
            for i := 0; i < len(macaddr_array); i++ {
                if i != index && macaddr_array[i] != "null" {
                    cmdStr = fmt.Sprintf("bridge fdb append to %s dst %s dev %s", macaddr_array[i], address[i][0], devName)
                    args = []string{"-c", cmdStr}
                    output, err = runcmd("sh", args, true)
                    if err != nil {
                        fmt.Println("Error:", cmdStr, output)
                    }
                    out = strings.Split(subnet[i], "/")
                    ip_address = out[0]
                    cmdStr = fmt.Sprintf("ip neigh add %s lladdr %s dev %s", ip_address, macaddr_array[i], devName)
                    args = []string{"-c", cmdStr}
                    output, err = runcmd("sh", args, true)
                    if err != nil {
                        fmt.Println("Error:", cmdStr, output)
                    }
                }
            }
        }
        done = 1
    }

    // Setup ip route to other VMs
    if num_nodes != number_nodes {
        args = []string{"route"}
        output, err = runcmd("ip", args, true)
        if err != nil {
            fmt.Println("Error:", cmdStr)
        }
        out = strings.Split(strings.TrimSuffix(output, "\n"), "\n")
        for i := 0; i < len(subnet); i++ {
            if i != index && subnet[i] != "" {
                out1 = strings.Split(subnet[i], "/")
                ip_address = out1[0]
                cmdStr = fmt.Sprintf("%s via %s dev %s onlink", subnet[i], ip_address, devName)
                match = false
                for j := 0; j < len(out); j++ {
                    if strings.Contains(out[j], cmdStr) {
                        match = true
                        break
                    }
                }
                if match == false {
                    args = []string{"route", "add", subnet[i], "via", ip_address, "dev", devName, "onlink"}
                    _, err = runcmd("ip", args, true)
                    if err != nil {
                        fmt.Println("Error: ip route add", subnet[i], "via", ip_address, "dev", devName, "onlink")
                    }
                }
            }
        }
    }

    args = []string{"-c", "kubectl get nodes --watch-only=true"}
    output, err = runcmd("sh", args, true)
    if err != nil {
        fmt.Println("Error:", output)
    }
    goto loop
}
