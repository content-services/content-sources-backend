## Prep your environment
1.  ensure rhel9 repos are enabled and snapshotted
```
make repos-import-rhel9
```
2. 
```
go run cmd/external-repos/main.go nightly-jobs
```
3.  Let those repositories snapshot

4.  Note that the current instructions do not work with content_guards enabled.  If `custom_repo_content_guards` is set to true in config.yaml, content access will be denied for custom repositories.

## Configure firewall
 You might need to adjust your firewall to allow access.  When running on linux, you can set your default firewall zone to "trusted":
```bash
sudo firewall-cmd --set-default-zone=trusted
```
To fully disable the firewall on linux:
```bash
sudo systemctl stop firewalld
```

## Registering a subscription-manager client to your local environment

This is assuming you are running a development environment on a Hypervisor (we'll refer to this as a Laptop to differentiate it from the client Virtual Machine). 

1. Install a RHEL 9 client as a VM
2. Find the IP Address the VM will access the host running the development environment:
```bash
ifconfig
```
or
```bash
ip list
```
for example, if a VM has an ip address of 192.168.122.58, the Host's IP address is typically 192.168.122.1

3.  On the Laptop add this to /etc/hosts:
```
192.168.122.1 pulp.content
```
4. Configure the client, within the VM:
```bash
subscription-manager config --server.hostname=subscription.rhsm.stage.redhat.com --server.port=443  --server.prefix=/subscription --server.insecure=0  --server.proxy_hostname=squid.corp.redhat.com  --server.proxy_port=3128
```
5. Register the client within the VM:
```bash
subscription-manager register
```
Username: admin
Password: admin

6. Fetch client UUID, within the VM:
```bash
$ subscription-manager identity
system identity: 97d9b21f-9b49-4eae-ade7-beb2b050dee1
```
7. Add custom repositories if required using UI or API
8. Create a content template via UI or API
9. Associate system to content template, within the content-sources-backend git repo:
```bash
go run cmd/candlepin/main.go add-system  $SYSTEM_IDENTITY $TEMPLATE_NAME 
```
10. refresh subscription-manager, on the VM to fetch new certs:
```bash
subscription-manager refresh
```
11.  Yum repolist on the VM to show 