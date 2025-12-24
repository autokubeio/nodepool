#!/bin/bash

# Debug cloud-init on OVHcloud instance
# Usage: ./debug-cloud-init.sh <instance-ip>

if [ -z "$1" ]; then
    echo "Usage: $0 <instance-ip>"
    echo "Example: $0 40.160.50.6"
    exit 1
fi

INSTANCE_IP=$1

echo "=== Checking cloud-init on instance $INSTANCE_IP ==="
echo ""

ssh ubuntu@$INSTANCE_IP << 'EOF'
echo "1. Cloud-init status:"
sudo cloud-init status --long
echo ""

echo "2. Check if cloud-init finished:"
sudo cloud-init status --wait
echo ""

echo "3. Cloud-init output log (last 50 lines):"
sudo tail -50 /var/log/cloud-init-output.log
echo ""

echo "4. Check for errors in cloud-init:"
sudo grep -i error /var/log/cloud-init-output.log | tail -20
echo ""

echo "5. Check if kubeadm is installed:"
which kubeadm
kubeadm version 2>/dev/null || echo "kubeadm not found"
echo ""

echo "6. Check if kubelet is running:"
sudo systemctl status kubelet --no-pager
echo ""

echo "7. Check user-data received:"
sudo cat /var/lib/cloud/instance/user-data.txt | head -20
echo ""

echo "8. Cloud-init stages:"
sudo cloud-init analyze show
EOF
