(ns golem.dsl.primitives.builtins
  "Built-in primitives: plan, implement, review, reflect, research, run-tests."
  (:require [golem.dsl.core :refer [defprimitive]]))

;; Each primitive's execute fn receives [state context adapter]
;; and returns a map of the writes keys.
;; For :session true primitives, the engine handles prompt rendering
;; and session spawning; the execute fn processes the session output.

(defprimitive plan
  "Analyze goal and produce step-by-step plan."
  {:reads [:goal] :writes [:plan] :session true}
  (fn [state context adapter]
    (:output context)))

(defprimitive implement
  "Write code, run tests, fix failures in a single session."
  {:reads [:goal :plan]
   :optional-reads [:reflection :review-feedback]
   :writes [:code :test-results]
   :session true}
  (fn [state context adapter]
    (:output context)))

(defprimitive review
  "Review code quality and correctness."
  {:reads [:code :test-results]
   :writes [:review-feedback]
   :session true}
  (fn [state context adapter]
    (:output context)))

(defprimitive reflect
  "Self-critique on specified state keys."
  {:reads [:code :test-results]
   :optional-reads [:plan :review-feedback]
   :writes [:reflection]
   :session true}
  (fn [state context adapter]
    (:output context)))

(defprimitive research
  "Gather technical context on topics."
  {:reads [:goal]
   :writes [:research-context]
   :session true}
  (fn [state context adapter]
    (:output context)))

(defprimitive run-tests
  "Execute tests locally without agent session."
  {:reads [:code]
   :writes [:test-results]
   :session false}
  (fn [state context adapter]
    (let [code (:code state)
          lang (or (:language code) "unknown")
          cmd (case lang
                "go" "go test ./..."
                "clojure" "clj -X:test"
                (str "echo 'Unknown language: " lang "'"))
          proc (.exec (Runtime/getRuntime) (into-array String ["sh" "-c" cmd]))
          exit (.waitFor proc)]
      {:test-results {:status (if (= 0 exit) :pass :fail)
                      :failures []}})))
