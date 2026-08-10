// Minimal shape CannedResponsePicker and CannedResponseManager need — a
// subset of the full API resource (which also has createdAt/updatedAt, see
// ../types.ts). Kept separate so both components can import/re-export the
// same type without depending on fields they don't use.
export interface CannedResponse {
  id: string
  title: string
  body: string
  tags: string[]
}
