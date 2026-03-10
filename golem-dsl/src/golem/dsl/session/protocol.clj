(ns golem.dsl.session.protocol)

(defprotocol SessionAdapter
  (spawn [this prompt working-dir opts]
    "Launch a session. Returns an opaque handle.")
  (await-result [this handle timeout-ms]
    "Block until session completes. Returns {:exit-code N}")
  (read-output [this handle]
    "Read session output. Returns map of state keys."))
