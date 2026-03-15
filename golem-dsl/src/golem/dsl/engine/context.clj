(ns golem.dsl.engine.context
  "Prompt template rendering from contract reads."
  (:require [stencil.core :as stencil]
            [clojure.java.io :as io]
            [golem.dsl.engine.state :as state]))

(defn- load-template [primitive-key]
  (let [path (str "prompts/" (name primitive-key) ".md")
        resource (io/resource path)]
    (when resource
      (slurp resource))))

(defn- template-for [primitive-key contract]
  (or (when-let [custom (:prompt-template contract)]
        (slurp custom))
      (load-template primitive-key)
      (throw (ex-info (str "No prompt template for " primitive-key)
                      {:primitive primitive-key}))))

(defn render-prompt
  "Render a prompt template with state keys from the contract."
  [primitive-key current-state contract]
  (let [template (template-for primitive-key contract)
        reads (:reads contract [])
        optional-reads (:optional-reads contract [])
        context (state/select-reads current-state reads optional-reads)
        ;; Convert to string keys for Mustache, stringify values for rendering
        string-context (into {} (map (fn [[k v]]
                                       [(name k) (if (string? v) v (pr-str v))])
                                     context))]
    (stencil/render-string template string-context)))
