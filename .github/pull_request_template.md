## Summary

- 

## Verification

- [ ] `go test ./...`
- [ ] `go build ./...`
- [ ] `cd apps/web && npm run build`
- [ ] Manual check if UI/API behavior changed

## Boundary Check

- [ ] Changes stay inside `wx_channel_insight`
- [ ] No analytics owner moved back into `wx_channel_hub`
- [ ] No direct dependency added on Hub SQLite analytics reads

## Notes

- 
