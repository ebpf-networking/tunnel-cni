package main

import (
    "fmt"
    "log"
    "os"
    "os/exec"
    "strings"
    "flag"
)

var Log *log.Logger

func init() {
    var logpath = "/var/log/tunnel-cni-plugin.log"

    flag.Parse()
    var file, err = os.OpenFile(logpath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
    if err != nil {
        panic(err)
    }
//    defer file.Close()
    Log = log.New(file, "", log.LstdFlags | log.Lmicroseconds | log.Lshortfile)
}

func runcmd(path string, args []string, debug bool) (output string, err error) {
    cmd := exec.Command(path, args...)

    var b []byte
    b, err = cmd.CombinedOutput()
    out := strings.TrimSuffix(string(b), "\n")

    if debug {
        Log.Println(strings.Join(cmd.Args[:], " "))
        if err != nil {
            Log.Println("Error: runcmd", out)
        }
    }
    return out, err
}

var cni_command, cni_containerid, cni_netns, cni_ifname, cni_path string

func getenv() {
    cni_command = os.Getenv("CNI_COMMAND")
    cni_containerid = os.Getenv("CNI_CONTAINERID")
    cni_netns = os.Getenv("CNI_NETNS")
    cni_ifname = os.Getenv("CNI_IFNAME")
    cni_path = os.Getenv("CNI_PATH")
}

func printenv(command, containerid, netns, ifname, path string) {
    Log.Println("CNI_CONTAINERID:", containerid, "\nCNI_NETNS:", netns, "\nCNI_IFNAME:", ifname, "\nCNI_PATH:", path)
}

func index2At(str, sub string, n int) (int, int) {
    i := 0
    j := 0
    for m := 1; m <= n; m++ {
        x := strings.Index(str[i:], sub)
        if x < 0 {
            break
        }
        i += x + 1
        if m == n {
            x = strings.Index(str[i:], sub)
            if x < 0 {
                return i-1, -1
            } else {
                j = i + x
                return i-1, j
            }
        }
    }
    return -1, -1
}

func main() {
    var output, network, subnet, cmdStr, myname, gw_ip, gw_cidr, con_cidr, container_ip string
    var out, args []string
    var ipaddr, hostdev, guestmac string
    var index, idx int
    var err error

    getenv()
    Log.Println(cni_command, "Start")

    filename := "/etc/cni/net.d/10-tunnel-cni-plugin.conf"
    datadir  := "/var/lib/cni/networks"

    printenv(cni_command, cni_containerid, cni_netns, cni_ifname, cni_path)

    switch cni_command {
    case "ADD":
        // Read the conf file
        cmdStr = fmt.Sprintf("cat %s", filename)
        args = []string{"-c", cmdStr}
        output, err = runcmd("sh", args, true)
        if err != nil {
            Log.Println("Error:", output)
        }
        index, idx = index2At(output, "\"", 7)
        myname = string(output[index+1:idx])
        index, idx = index2At(output, "\"", 15)
        network = string(output[index+1:idx])
        index, idx = index2At(output, "\"", 19)
        subnet = string(output[index+1:idx])
        Log.Println(myname, network, subnet)

        // Get the gw_ip and gw_cidr
        out = strings.Split(subnet, "/")
        ipaddr = out[0]
        mask_size := out[1]
        slen := len(ipaddr)
        if slen > 0 && ipaddr[slen - 1] == '0' {
            gw_ip = ipaddr[:slen - 1] + "1"
        }
        gw_cidr = gw_ip + "/" + mask_size

        // Link cni_netns to /var/run/netns
        cmdStr = fmt.Sprintf("ln -sfT %s /var/run/netns/%s", cni_netns, cni_containerid)
        args = []string{"-c", cmdStr}
        output, err = runcmd("sh", args, true)
        if err != nil {
            Log.Println("Error: ln -sfT", output)
        }

        // Call the bridge plugin
        cmdStr = fmt.Sprintf("echo \"{\\\"cniVersion\\\": \\\"0.3.1\\\", \\\"type\\\": \\\"tunnel-cni\\\", \\\"name\\\": \\\"%s\\\", \\\"ipam\\\": { \\\"type\\\": \\\"host-local\\\", \\\"subnet\\\": \\\"%s\\\" }}\" | CNI_COMMAND=ADD CNI_CONTAINERID=%s CNI_NETNS=%s CNI_IFNAME=%s CNI_PATH=%s /opt/cni/bin/bridge", myname, subnet, cni_containerid, cni_netns, cni_ifname, cni_path)
        args = []string{"-c", cmdStr}
        output, err = runcmd("sh", args, true)
        if err != nil {
            Log.Println("Error:", output)
        }
        index, idx = index2At(output, "\"", 45)
        con_cidr = string(output[index+1:idx])
        out = strings.Split(con_cidr, "/")
        container_ip = out[0]
        index, idx = index2At(output, "\"", 17)
        hostdev = string(output[index+1:idx])
        index, idx = index2At(output, "\"", 29)
        guestmac = string(output[index+1:idx])
        Log.Println(gw_ip, gw_cidr, hostdev, container_ip, guestmac)

        // Add the ip route entry for the container via the gateway
        cmdStr = fmt.Sprintf("ip netns exec %s ip route add default via %s dev %s", cni_containerid, gw_ip, cni_ifname)
        args = []string{"-c", cmdStr}
        output, err = runcmd("sh", args, true)
        if err != nil {
            Log.Println("Error: ip netns exec", output)
        }
        cmdStr = fmt.Sprintf("ip netns exec %s ip route add %s via %s dev %s", cni_containerid, network, gw_ip, cni_ifname)
        args = []string{"-c", cmdStr}
        output, err = runcmd("sh", args, true)
        if err != nil {
            Log.Println("Error: ip netns exec", output)
        }

        // Output the needed information through stdout
        cmdStr = fmt.Sprintf("{\n    \"cniVersion\": \"0.3.1\",\n    \"interfaces\": [\n        {\n            \"name\": \"eth0\",\n            \"mac\": \"%s\",\n            \"sandbox\": \"%s\"\n        }\n    ],\n    \"ips\": [\n        {\n            \"version\": \"4\",\n            \"address\": \"%s/%s\",\n            \"gateway\": \"%s\",\n            \"interface\": 0\n        }\n    ]\n}", guestmac, cni_netns, container_ip, mask_size, gw_ip)
        fmt.Println(cmdStr)

        // Mount /sys/fs/bpf and add XDP code
/*
        args = []string{"-s", "/sys/fs/bpf", "/proc/mounts"}
        output, err = runcmd("grep", args, true)
        if err != nil && len(output) == 0 {
            args = []string{"-t", "bpf", "bpffs", "/sys/fs/bpf"}
            output, err = runcmd("mount", args, true)
            if err != nil {
                Log.Println("Error: mount -t bpf bpffs /sys/fs/bpf", output)
            }
        }
        args = []string{"-c", "ulimit -l unlimited"}
        output, err = runcmd("sh", args, true)
        if err != nil {
            Log.Println("Error: ulimit -l unlimited", output)
        }
        cmdStr = fmt.Sprintf("/opt/cni/xdp/xdp-loader load -p /sys/fs/bpf/%s -s xdp_stats %s /opt/cni/xdp/xdp_kern.o", hostdev, hostdev)
        args = []string{"-c", cmdStr}
        output, err = runcmd("sh", args, true)
        if err != nil {
            Log.Println("Error: xdp-loader load", output)
        }
*/
    case "DEL":
        cmdStr = fmt.Sprintf("cat %s", filename)
        args = []string{"-c", cmdStr}
        output, err = runcmd("sh", args, true)
        if err != nil {
            Log.Println("Error:", output)
        }
        index, idx = index2At(output, "\"", 7)
        myname = string(output[index+1:idx])

        cmdStr = fmt.Sprintf("ip netns exec %s ip addr show eth0 | awk '/inet / {print $2}' | awk -F\"/\" '{print $1}'", cni_containerid)
        args = []string{"-c", cmdStr}
        ipaddr, err = runcmd("sh", args, true)
        if err != nil {
            Log.Println("Error: ip netns exec", ipaddr)
        }
        Log.Println(myname, ipaddr)
        cmdStr = fmt.Sprintf("rm %s/%s/%s", datadir, myname, ipaddr)
        args = []string{"-c", cmdStr}
        output, err = runcmd("sh", args, true)
        if err != nil {
            Log.Println("Error:", output)
        }
    case "VERSION":
        fmt.Println("{\n    \"cniVersion\": \"0.3.1\",\n    \"supportedVersions\": [ \"0.3.0\", \"0.3.1\", \"0.4.0\" ]\n}")
    case "GET":
    default: 
    }
    Log.Println(cni_command, "End")
}
