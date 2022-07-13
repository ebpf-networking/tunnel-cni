# Setting up the tunnel-cni plugin in a Kubernetes cluster

This is the tunnel-cni plugin with its DaemonSet to setup the tunnel-cni plugin in a kubernetes cluster. 
The tunnel-cni uses tunneling backend for pod-to-pod communications between pods in different nodes. There are 
two tunneling technologies, vxlan and geneve. The default backend is vxlan. Geneve tunneling will be available soon.
In addition to the `backend` technology choice, the vni `id`, the `dstport`, and the `qlen` of the interface device
default to "1", "8472", and "0". They can be changed through command line arguments such as "-backend=vxlan", "-id=1", 
"-dstport=8472", and "-qlen=0".

The tunnel-cni uses the 
`bridge` plugin to create veth pairs between the bridge and the containers. The `bridge` plugin calls the `host-local` plugin for 
IP Address Management (IPAM). The log file at `/var/log/tunnel-cni-plugin.log` gives timestamps
in microseconds. The steps are listed below.

## Create a cluster

We first need a K8s cluster, which can be done by any of the following approaches:
- [Kubeadm](docs/kubeadm.md)
- [Kind](docs/kind.md)

## Install the bash-cni plugin

You can pull the `cericwu/tunnel` image first. Or you could build your own image using the `docker build -t <your_image_name> .` command. Here we use the `cericwu/bashcni` image. Usually we don't need to pull the image. It is used here to make sure that you have access to the docker repository. If not, you need to install docker using the command `sudo apt install docker-ce docker-ce-cli docker.io`.

```
$ docker login
...
Login Succeeded
$ docker pull cericwu/tunnel
...
docker.io/cericwu/tunnel:latest
```

Run the `kubectl apply -f tunnel-ds.yml` command to install the tunnel-cni plugin.

```
$ kubectl apply -f tunnel-ds.yml
clusterrolebinding.rbac.authorization.k8s.io/admin-user created
serviceaccount/admin-user created
daemonset.apps/tunnel created
$ ls -l /etc/cni/net.d
-rw-r--r-- 1 root root 140 Jul 12 21:00 10-tunnel-cni-plugin.conf
$ ls -l /opt/cni/bin/tunnel-cni
-rwxr-xr-x 1 root root 2189707 Jul 12 21:00 /opt/cni/bin/tunnel-cni
$ ls -l /opt/cni/bin/host-local
-rwxr-xr-x 1 root root 3614480 May  6 11:42 /opt/cni/bin/host-local
```

The output shows the tunnel daemonset is created with its service account and cluster role binding.
The tunnel-cni configuration file `10-tunnel-cni-plugin.conf` is automatically created
and installed in the `/etc/cni/net.d` directory on each node.
The tunnel-cni binary executable is also installed in the `/opt/cni/bin` directory on each node. It uses
the `host-local` plugin for IP Address Management (IPAM) for generating IP addresses.
The daemonset pods are created in the namespace `kube-system`.
We can run the following command to check on them.


```
$ kubectl get pods -n kube-system
```

The output will show several tunnel pods running, one on each node.

## Deploy a few pods in the kubernetes cluster

We want to deploy a few pods to test the bash-cni plugin.


```
$ kubectl apply -f deploy_monty.yml
pod/monty-vm2 created
pod/monty-vm3 created
$ kubectl run -it --image=busybox pod1 -- sh
$ kubectl get pods -o wide
NAME        READY  STATUS    RESTARTS  AGE  IP           NODE
monty-vm2   1/1    Running   0         5m   10.244.1.4   vm2
monty-vm3   1/1    Running   0         5m   10.244.2.3   vm3
pod1        1/1    Running   0         3m   10.244.2.4   vm3
```

It shows that the pods are installed successfully with the tunnel-cni plugin. Their IP addresses are also listed in the output.

## Test the connectivities of the pods

We can test the connectivities of the pods as shown below.

```
$ kubectl exec -it pod1 -- sh
/ # ping 10.244.2.3                     // Pod on the same VM
/ # ping 10.244.1.4                     // Pod on a different VM
/ # ping 192.168.122.135                // IP address of the master node
/ # ping 192.168.122.160                // IP address of the host VM
/ # ping 192.168.122.158                // IP address of the other VM
/ # ping 185.125.190.29                 // IP address of ubuntu.com
```

The connection to all the above should be OK after the bashcni daemonset is installed.

## Deleting the daemonset

Deleting the daemonset won't affect the function of the tunnel-cni plugin.

```
$ kubectl delete -f tunnel-ds.yml
clusterrolebinding.rbac.authorization.k8s.io "admin-user" deleted
serviceaccount "admin-user" deleted
daemonset.apps "tunnel" deleted
$ ls -l /etc/cni/net.d
-rw-r--r-- 1 root root 140 Jul 12 21:00 10-tunnel-cni-plugin.conf
$ ls -l /opt/cni/bin/tunnel-cni
-rwxr-xr-x 1 root root 2189707 Jul 12 21:00 /opt/cni/bin/tunnel-cni
$ ls -l /opt/cni/bin/host-local
-rwxr-xr-x 1 root root 3614480 May  6 11:42 /opt/cni/bin/host-local
```

This is because the bridge, iptables, and ip route entries remain unchanged
after the tunnel daemonset is deleted.
