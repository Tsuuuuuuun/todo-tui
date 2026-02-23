// TODO: Migrate to TypeScript
const express = require("express");

const app = express();

// FIXME: CORS headers are too permissive
app.use((req, res, next) => {
  res.header("Access-Control-Allow-Origin", "*");
  next();
});

// TODO: Add rate limiting middleware
app.get("/api/users", (req, res) => {
  // HACK: Hardcoded user list for now
  res.json([{ id: 1, name: "Alice" }]);
});

// NOTE: Port should come from environment variable
const PORT = 3000;
app.listen(PORT);
