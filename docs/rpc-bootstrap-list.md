# M0 RPC Bootstrap List (live capture)

Total requests: 47

| Count | Path |
|---:|---|
| 20 | /_route/api_server/exa.seat_management_pb.SeatManagementService/GetUserStatus |
| 9 | /_route/api_server/exa.auth_pb.AuthService/GetUserJwt |
| 5 | /_route/api_server/exa.api_server_pb.ApiServerService/CheckChatCapacity |
| 5 | /_route/api_server/exa.api_server_pb.ApiServerService/CheckUserMessageRateLimit |
| 3 | /_route/api_server/exa.api_server_pb.ApiServerService/Ping |
| 2 | /_route/api_server/exa.api_server_pb.ApiServerService/GetModelStatuses |
| 1 | /_route/api_server/exa.product_analytics_pb.ProductAnalyticsService/RecordAnalyticsEvent |
| 1 | /_route/api_server/exa.cascade_plugins_pb.CascadePluginsService/GetAllAcpRegistries |
| 1 | /_route/api_server/exa.api_server_pb.ApiServerService/GetDefaultWorkflowTemplates |

## Notes
- Protocol: Connect-RPC (Connect-Protocol-Version: 1)
- Content-Type: mostly pplication/proto
- LS real process uses local URL via wrapper
- Parent process cmdline may still show original URL (wrapper process)
