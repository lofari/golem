(ns golem.dsl.sync
  "Sync DSL state to .ctx/state.yaml for golem status command."
  (:require [clojure.string :as str]
            [clojure.java.io :as io]))

(defn- yaml-escape [s]
  (if (and (string? s) (re-find #"[:\n\"]" s))
    (str "\"" (str/escape s {\" "\\\"" \newline "\\n"}) "\"")
    (str s)))

(defn project-state-yaml
  "Convert DSL state to a state.yaml compatible string."
  [dsl-state agent-name phase]
  (str "project:\n"
       "  name: \"\"\n"
       "  summary: \"\"\n"
       "status:\n"
       "  current_focus: " (yaml-escape (str (:goal dsl-state ""))) "\n"
       "  phase: " (yaml-escape phase) "\n"
       "  last_session: " (yaml-escape (str agent-name)) "\n"
       "tasks: []\n"
       "decisions: []\n"
       "pitfalls: []\n"))

(defn write-state-yaml!
  "Write state.yaml to the given path."
  [path dsl-state agent-name phase]
  (io/make-parents path)
  (spit path (project-state-yaml dsl-state agent-name phase)))
