(ns golem.dsl.errors.diagnostic
  (:require [clojure.java.io :as io]))

(defn save-diagnostic!
  "Write error diagnostic to runs/<run-id>/errors/"
  [base-dir run-id version diagnostic]
  (let [dir (io/file base-dir "runs" (name run-id) "errors")
        file (io/file dir (str "error-v" version ".edn"))]
    (.mkdirs dir)
    (spit file (pr-str diagnostic))))
