#!/bin/bash

# Wait for nodes to start
echo "Waiting for nodes to initialize..."
sleep 5

# Create a dummy test file
echo "This is a test file for the HydraStore distributed cluster. Random: $RANDOM$RANDOM" > test_file.txt

echo "Authenticating as admin to get JWT..."
LOGIN_RESP=$(curl -s -X POST -d "username=admin&password=adminpass" "http://localhost:8080/v1/auth/login")
JWT=$(echo $LOGIN_RESP | grep -o '"token":"[^"]*' | cut -d'"' -f4)

if [ -z "$JWT" ]; then
    echo "ERROR: Failed to authenticate. Response: $LOGIN_RESP"
    exit 1
fi
echo "Authenticated successfully."

echo "Uploading file to Node 1 (10.5.0.11:8080)..."
UPLOAD_RESPONSE=$(curl -s -X POST --data-binary @test_file.txt "http://localhost:8080/v1/files?key=test_file.txt" -H "Authorization: Bearer $JWT")

echo "Upload Response: $UPLOAD_RESPONSE"

# Extract CID from response (basic parsing)
CID=$(echo $UPLOAD_RESPONSE | grep -o '"cid":"[^"]*' | cut -d'"' -f4)

if [ -z "$CID" ]; then
    echo "Upload failed, could not extract CID."
    exit 1
fi

echo "File uploaded successfully with CID: $CID"

echo "Waiting for replication to complete..."
sleep 2

echo "Deleting local copy from Node 1 to force network retrieval..."
docker exec hydrastore_node1 rm -rf /root/node1_network/node1

echo "Downloading file from Node 1 (10.5.0.11:8080) - should fetch from network..."
curl -s "http://localhost:8080/v1/files/test_file.txt" -H "Authorization: Bearer $JWT" > downloaded_file.txt

# Verify content
if cmp -s test_file.txt downloaded_file.txt; then
    echo "SUCCESS: Downloaded file matches original file!"
    echo "Downloaded file: $(cat downloaded_file.txt)"
    echo "Original file: $(cat test_file.txt)"
else
    echo "ERROR: Downloaded file does not match original file."
    exit 1
fi

rm test_file.txt downloaded_file.txt
echo "Cluster test passed successfully!"
