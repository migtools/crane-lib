# Compatibility Matrix
| Transfer      | Transport     | Endpoint      | State                   |
| ------------- | ------------- | ------------- | ----------------------- |
| rsync         | null          | route         | incompatible            |
| rsync         | null          | load balancer | functional but insecure |
| rsync         | stunnel       | route         | functional              |
| rsync         | stunnel       | load balancer | functional              |
| rclone        | null          | route         | incompatible            |
| rclone        | null          | load balancer | functional but insecure |
| rclone        | stunnel       | route         | functional              |
| rclone        | stunnel       | load balancer | functional              |
