// Package messaging owns composing a message and everything the sender sees before
// confirming a batch.
//
// Scope (see description.md "Tworzenie wiadomosci" and "Dlugosc wiadomosci SMS"):
//   - draft: e-mail subject + one shared body used by BOTH channels,
//   - personalisation placeholders, rendered identically for e-mail and SMS,
//   - GSM-7/UCS-2 detection and SMS part counting per recipient,
//   - pre-send summary: body length, parts per recipient, recipient count, total parts, cost,
//   - preview and the final confirmation step.
//
// Invariant: the e-mail body and the SMS body of a given recipient are byte-identical.
// Nothing here may shorten, summarise or reformat the text.
//
// Owner: DEV B.
package messaging
