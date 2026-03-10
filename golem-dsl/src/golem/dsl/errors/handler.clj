(ns golem.dsl.errors.handler
  (:require [golem.dsl.errors.types :as types]))

(defn resolve-handler
  "Find handler for error type. Priority: step > agent > default."
  [error-type step-handlers agent-handlers]
  (or (get step-handlers error-type)
      (get agent-handlers error-type)
      (get types/defaults error-type)))

(defn should-retry?
  "Check if we should retry given handler config and current attempt."
  [handler attempt]
  (boolean
   (and (#{:retry :re-run} (:action handler))
        (< attempt (or (:max handler) 3)))))

(defn amend-prompt
  "Amend prompt for re-run with error context."
  [original-prompt error-info handler]
  (let [hint (or (:hint handler) "Previous attempt failed. Check output format.")
        error-msg (or (:message error-info) "Unknown error")]
    (str original-prompt
         "\n\n## Error from Previous Attempt\n"
         error-msg
         "\n\n## Hint\n"
         hint)))
