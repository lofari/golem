(ns golem.dsl.resolve
  "Resolve agent names to file paths. Project-local takes priority over built-in."
  (:require [clojure.java.io :as io]
            [clojure.string :as str]))

(def builtin-agents
  [{:name "build-feature" :desc "Plan → implement → review loop" :file "build_feature.clj"}
   {:name "fix-bug"       :desc "Research → fix → test loop"     :file "fix_bug.clj"}
   {:name "write-docs"    :desc "Documentation generator"        :file "write_docs.clj"}
   {:name "review"        :desc "Single-pass code review"        :file "review.clj"}])

(defn- builtin-path
  "Find path to a built-in agent file relative to the agents/ directory."
  [agent-name]
  (let [entry (some #(when (= agent-name (:name %)) %) builtin-agents)]
    (when entry
      (let [;; Try classpath resource first
            resource (io/resource (str "agents/" (:file entry)))]
        (if resource
          (.getPath resource)
          ;; Fall back to agents/ dir relative to project
          (let [f (io/file "agents" (:file entry))]
            (when (.exists f) (.getPath f))))))))

(defn resolve-agent
  "Resolve agent name to file path. Project-local (agents-dir) takes priority."
  [agent-name agents-dir]
  (let [local-file (when agents-dir
                     (let [f (io/file agents-dir (str agent-name ".clj"))]
                       (when (.exists f) (.getPath f))))]
    (or local-file (builtin-path agent-name))))

(defn list-agents
  "List all available agents with source."
  [agents-dir]
  (let [builtins (map #(assoc % :source :built-in) builtin-agents)
        locals (when agents-dir
                 (let [dir (io/file agents-dir)]
                   (when (.isDirectory dir)
                     (->> (.listFiles dir)
                          (filter #(str/ends-with? (.getName %) ".clj"))
                          (map (fn [f]
                                 {:name (str/replace (.getName f) ".clj" "")
                                  :desc (.getPath f)
                                  :source :project}))))))]
    (concat locals builtins)))
